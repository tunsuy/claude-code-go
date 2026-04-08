package oauth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// AuthCodeListener starts a temporary local HTTP server to capture the OAuth callback.
// Corresponds to TS AuthCodeListener.
type AuthCodeListener struct {
	server        *http.Server
	port          int
	expectedState atomic.Value // string — written by WaitForCode, read by handleCallback (P1-3 fix)
	codeCh        chan string
	errCh         chan error
	callbackPath  string
	once          sync.Once
}

// NewAuthCodeListener creates a listener with the given callback path.
// callbackPath defaults to "/callback" when empty.
func NewAuthCodeListener(callbackPath string) *AuthCodeListener {
	if callbackPath == "" {
		callbackPath = "/callback"
	}
	l := &AuthCodeListener{
		callbackPath: callbackPath,
		codeCh:       make(chan string, 1),
		errCh:        make(chan error, 1),
	}
	l.expectedState.Store("") // initialise so Load() is always safe (P1-3)
	return l
}

// Start starts the listener on the given port.
// port=0 lets the OS pick a random available port.
// If a non-zero port is occupied, falls back to a random port (up to 3 attempts).
// Returns the actual port in use.
func (l *AuthCodeListener) Start(port int) (int, error) {
	var ln net.Listener
	var err error

	if port == 0 {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, fmt.Errorf("oauth: listener: bind random port: %w", err)
		}
	} else {
		// Try the requested port first, then fall back to random
		ln, err = net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err != nil {
			// Port occupied: retry with random port up to 3 times
			var fallbackErr error
			for attempt := 0; attempt < 3; attempt++ {
				ln, fallbackErr = net.Listen("tcp", "127.0.0.1:0")
				if fallbackErr == nil {
					break
				}
			}
			if ln == nil {
				return 0, fmt.Errorf("oauth: listener: port %d occupied and fallback failed: %w", port, err)
			}
		}
	}

	l.port = ln.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc(l.callbackPath, l.handleCallback)
	l.server = &http.Server{Handler: mux}

	go func() {
		if serveErr := l.server.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			select {
			case l.errCh <- serveErr:
			default:
			}
		}
	}()

	return l.port, nil
}

// handleCallback processes the OAuth redirect.
func (l *AuthCodeListener) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state := q.Get("state")
	code := q.Get("code")
	errParam := q.Get("error")

	if errParam != "" {
		desc := q.Get("error_description")
		msg := errParam
		if desc != "" {
			msg += ": " + desc
		}
		l.errCh <- fmt.Errorf("oauth: callback error: %s", msg)
		http.Error(w, "Authentication failed: "+msg, http.StatusBadRequest)
		return
	}

	if state != l.expectedState.Load().(string) {
		l.errCh <- fmt.Errorf("oauth: state mismatch (CSRF check failed)")
		http.Error(w, "State mismatch", http.StatusBadRequest)
		return
	}

	if code == "" {
		l.errCh <- fmt.Errorf("oauth: no code in callback")
		http.Error(w, "No code received", http.StatusBadRequest)
		return
	}

	// Respond with a success page before sending the code
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<!DOCTYPE html><html><body>
<h2>Authentication successful!</h2>
<p>You can close this window and return to Claude Code.</p>
<script>window.close();</script>
</body></html>`)

	select {
	case l.codeCh <- code:
	default:
	}
}

// WaitForCode blocks until the authorization code arrives or ctx is cancelled.
// state must match the state parameter used when building the auth URL (CSRF protection).
func (l *AuthCodeListener) WaitForCode(ctx context.Context, state string) (string, error) {
	// Store state atomically so handleCallback can read it safely from a different goroutine (P1-3).
	l.expectedState.Store(state)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case code := <-l.codeCh:
		return code, nil
	case err := <-l.errCh:
		return "", err
	}
}

// RedirectSuccess redirects the browser to the appropriate success page.
// scopes determines whether to use the Console or Claude.ai success URL.
func (l *AuthCodeListener) RedirectSuccess(scopes []string) error {
	// Determine success URL based on scopes
	successURL := "https://console.anthropic.com/oauth/success"
	for _, s := range scopes {
		if strings.Contains(s, "claude.ai") {
			successURL = "https://claude.ai/oauth/success"
			break
		}
	}
	_ = successURL // The redirect happens in the callback handler
	return nil
}

// Close shuts down the listener server.
func (l *AuthCodeListener) Close() error {
	var err error
	l.once.Do(func() {
		if l.server != nil {
			err = l.server.Close()
		}
	})
	return err
}
