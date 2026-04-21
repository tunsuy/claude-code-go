package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ─── GenerateCodeVerifier / GenerateCodeChallenge / GenerateState ─────────────

func TestGenerateCodeVerifier_Length(t *testing.T) {
	v, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(v) == 0 {
		t.Error("expected non-empty verifier")
	}
	// base64url of 32 bytes = 43 chars (no padding)
	if len(v) != 43 {
		t.Errorf("expected 43 chars, got %d", len(v))
	}
}

func TestGenerateCodeVerifier_Uniqueness(t *testing.T) {
	v1, _ := GenerateCodeVerifier()
	v2, _ := GenerateCodeVerifier()
	if v1 == v2 {
		t.Error("verifiers should be unique")
	}
}

func TestGenerateCodeChallenge_S256RoundTrip(t *testing.T) {
	verifier := "test-verifier-string"
	challenge := GenerateCodeChallenge(verifier)

	// Manually compute expected S256 challenge
	h := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(h[:])
	if challenge != expected {
		t.Errorf("expected %q, got %q", expected, challenge)
	}
}

func TestGenerateState(t *testing.T) {
	s, err := GenerateState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s) != 43 {
		t.Errorf("expected 43 chars, got %d", len(s))
	}
	s2, _ := GenerateState()
	if s == s2 {
		t.Error("states should be unique")
	}
}

// ─── AES encryption round-trip ────────────────────────────────────────────────

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	plaintext := []byte(`{"access_token":"tok","refresh_token":"ref"}`)
	ct, err := encryptTokenFile(plaintext)
	if err != nil {
		t.Fatalf("encrypt error: %v", err)
	}
	pt, err := decryptTokenFile(ct)
	if err != nil {
		t.Fatalf("decrypt error: %v", err)
	}
	if string(pt) != string(plaintext) {
		t.Errorf("expected %q, got %q", plaintext, pt)
	}
}

func TestDecryptTokenFile_TooShort(t *testing.T) {
	_, err := decryptTokenFile([]byte("short"))
	if err == nil {
		t.Error("expected error for too-short ciphertext")
	}
}

func TestDecryptTokenFile_WrongVersion(t *testing.T) {
	// Build a buffer with wrong version prefix
	buf := make([]byte, 4+32+12+16+1)
	buf[3] = 0xFF // version = 4278190080
	_, err := decryptTokenFile(buf)
	if err == nil {
		t.Error("expected error for wrong version")
	}
}

// ─── MemoryStore ──────────────────────────────────────────────────────────────

func TestMemoryStore_LoadEmpty(t *testing.T) {
	m := &MemoryStore{}
	tokens, err := m.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens != nil {
		t.Error("expected nil tokens")
	}
}

func TestMemoryStore_SaveAndLoad(t *testing.T) {
	m := &MemoryStore{}
	want := &OAuthTokens{AccessToken: "tok1", RefreshToken: "ref1"}
	if err := m.Save(want); err != nil {
		t.Fatalf("save error: %v", err)
	}
	got, err := m.Load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if got.AccessToken != want.AccessToken {
		t.Errorf("expected %q, got %q", want.AccessToken, got.AccessToken)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	m := &MemoryStore{}
	_ = m.Save(&OAuthTokens{AccessToken: "tok"})
	_ = m.Delete()
	tokens, err := m.Load()
	if err != nil || tokens != nil {
		t.Errorf("expected nil after delete, got err=%v tokens=%v", err, tokens)
	}
}

func TestMemoryStore_SaveReturnsCopy(t *testing.T) {
	m := &MemoryStore{}
	original := &OAuthTokens{AccessToken: "original"}
	_ = m.Save(original)
	original.AccessToken = "mutated"

	got, _ := m.Load()
	if got.AccessToken != "original" {
		t.Error("expected immutable copy in store")
	}
}

// ─── FileStore ────────────────────────────────────────────────────────────────

func TestFileStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	fs := &FileStore{path: filepath.Join(dir, "tokens.enc")}

	tokens := &OAuthTokens{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		ExpiresAt:    time.Now().Add(time.Hour).UnixMilli(),
		Scopes:       []string{"org:inference"},
	}

	if err := fs.Save(tokens); err != nil {
		t.Fatalf("save error: %v", err)
	}

	loaded, err := fs.Load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if loaded.AccessToken != tokens.AccessToken {
		t.Errorf("expected %q, got %q", tokens.AccessToken, loaded.AccessToken)
	}
	if loaded.RefreshToken != tokens.RefreshToken {
		t.Errorf("expected %q, got %q", tokens.RefreshToken, loaded.RefreshToken)
	}
}

func TestFileStore_LoadNonExistent(t *testing.T) {
	dir := t.TempDir()
	fs := &FileStore{path: filepath.Join(dir, "nonexistent.enc")}
	tokens, err := fs.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens != nil {
		t.Error("expected nil tokens")
	}
}

func TestFileStore_Delete(t *testing.T) {
	dir := t.TempDir()
	fs := &FileStore{path: filepath.Join(dir, "tokens.enc")}
	_ = fs.Save(&OAuthTokens{AccessToken: "tok"})
	if err := fs.Delete(); err != nil {
		t.Fatalf("delete error: %v", err)
	}
	tokens, err := fs.Load()
	if err != nil || tokens != nil {
		t.Errorf("expected nil after delete, got err=%v tokens=%v", err, tokens)
	}
}

func TestFileStore_DeleteNonExistent(t *testing.T) {
	dir := t.TempDir()
	fs := &FileStore{path: filepath.Join(dir, "nofile.enc")}
	if err := fs.Delete(); err != nil {
		t.Errorf("expected no error deleting nonexistent, got %v", err)
	}
}

// ─── IsTokenExpired ───────────────────────────────────────────────────────────

func TestIsTokenExpired_Expired(t *testing.T) {
	past := time.Now().Add(-10 * time.Minute).UnixMilli()
	if !IsTokenExpired(past) {
		t.Error("expected expired=true for past timestamp")
	}
}

func TestIsTokenExpired_Valid(t *testing.T) {
	future := time.Now().Add(1 * time.Hour).UnixMilli()
	if IsTokenExpired(future) {
		t.Error("expected expired=false for future timestamp")
	}
}

func TestIsTokenExpired_NearExpiry(t *testing.T) {
	// Within 5 minute buffer → considered expired
	nearFuture := time.Now().Add(3 * time.Minute).UnixMilli()
	if !IsTokenExpired(nearFuture) {
		t.Error("expected expired=true for token expiring within buffer")
	}
}

// ─── singleflightGroup ────────────────────────────────────────────────────────

func TestSingleflightGroup_Basic(t *testing.T) {
	var g singleflightGroup
	val, err, shared := g.Do("key", func() (interface{}, error) {
		return "result", nil
	})
	if err != nil || val != "result" || shared {
		t.Errorf("unexpected: val=%v err=%v shared=%v", val, err, shared)
	}
}

func TestSingleflightGroup_Deduplication(t *testing.T) {
	var g singleflightGroup
	var calls int
	barrier := make(chan struct{})

	var wg sync.WaitGroup
	results := make([]interface{}, 3)
	errs := make([]error, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-barrier
			results[i], errs[i], _ = g.Do("key", func() (interface{}, error) {
				calls++
				time.Sleep(20 * time.Millisecond)
				return "shared", nil
			})
		}(i)
	}

	close(barrier)
	wg.Wait()

	// All should succeed
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, err)
		}
	}
}

func TestSingleflightGroup_PanicRecovery(t *testing.T) {
	var g singleflightGroup
	_, err, _ := g.Do("panic-key", func() (interface{}, error) {
		panic("test panic")
	})
	if err == nil {
		t.Error("expected error from panic recovery")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Errorf("expected 'panic' in error message, got %q", err.Error())
	}
}

func TestSingleflightGroup_ConcurrentSeparateKeys(t *testing.T) {
	var g singleflightGroup
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i)
			val, err, _ := g.Do(key, func() (interface{}, error) {
				return key, nil
			})
			if err != nil || val != key {
				t.Errorf("unexpected: val=%v err=%v", val, err)
			}
		}(i)
	}
	wg.Wait()
}

// ─── TokenManager ─────────────────────────────────────────────────────────────

func TestTokenManager_CheckAndRefresh_NoTokens(t *testing.T) {
	store := &MemoryStore{}
	tm := NewTokenManager(store, nil)
	tokens, err := tm.CheckAndRefreshIfNeeded(context.Background())
	if err != nil || tokens != nil {
		t.Errorf("expected (nil, nil), got tokens=%v err=%v", tokens, err)
	}
}

func TestTokenManager_CheckAndRefresh_ValidToken(t *testing.T) {
	store := &MemoryStore{}
	valid := &OAuthTokens{
		AccessToken:  "valid",
		RefreshToken: "ref",
		ExpiresAt:    time.Now().Add(time.Hour).UnixMilli(),
	}
	_ = store.Save(valid)

	tm := NewTokenManager(store, nil)
	tokens, err := tm.CheckAndRefreshIfNeeded(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens.AccessToken != "valid" {
		t.Errorf("expected 'valid', got %q", tokens.AccessToken)
	}
}

func TestTokenManager_CheckAndRefresh_ExpiredToken_Refreshes(t *testing.T) {
	// Mock HTTP server for token refresh
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := OAuthTokenExchangeResponse{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			ExpiresIn:    3600,
			Scope:        "org:inference",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	store := &MemoryStore{}
	expired := &OAuthTokens{
		AccessToken:  "old",
		RefreshToken: "old-ref",
		ExpiresAt:    time.Now().Add(-time.Hour).UnixMilli(), // expired
		Scopes:       []string{"org:inference"},
	}
	_ = store.Save(expired)

	cfg := DefaultOAuthConfig()
	cfg.TokenURL = server.URL
	oauthClient := NewClient(cfg, store, server.Client())
	tm := NewTokenManager(store, oauthClient)

	tokens, err := tm.CheckAndRefreshIfNeeded(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens.AccessToken != "new-access" {
		t.Errorf("expected 'new-access', got %q", tokens.AccessToken)
	}
}

func TestTokenManager_HandleOAuth401_TokenAlreadyRefreshed(t *testing.T) {
	store := &MemoryStore{}
	current := &OAuthTokens{AccessToken: "current-token", RefreshToken: "ref"}
	_ = store.Save(current)

	tm := NewTokenManager(store, nil)
	// If failedToken != currentToken, no refresh needed
	err := tm.HandleOAuth401Error(context.Background(), "different-old-token")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

// ─── AuthCodeListener ─────────────────────────────────────────────────────────

func TestAuthCodeListener_WaitForCode_CSRFMismatch(t *testing.T) {
	l := NewAuthCodeListener("/callback")
	port, err := l.Start(0)
	if err != nil {
		t.Fatalf("start error: %v", err)
	}
	defer l.Close()

	// Send request with wrong state
	go func() {
		_, _ = http.Get(fmt.Sprintf("http://localhost:%d/callback?state=wrong&code=mycode", port))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = l.WaitForCode(ctx, "correct-state")
	if err == nil {
		t.Error("expected CSRF error, got nil")
	}
}

func TestAuthCodeListener_WaitForCode_HappyPath(t *testing.T) {
	l := NewAuthCodeListener("/callback")
	port, err := l.Start(0)
	if err != nil {
		t.Fatalf("start error: %v", err)
	}
	defer l.Close()

	state := "test-state-123"
	go func() {
		time.Sleep(20 * time.Millisecond)
		_, _ = http.Get(fmt.Sprintf("http://localhost:%d/callback?state=%s&code=auth-code-456", port, state))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	code, err := l.WaitForCode(ctx, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "auth-code-456" {
		t.Errorf("expected 'auth-code-456', got %q", code)
	}
}

func TestAuthCodeListener_WaitForCode_ErrorParam(t *testing.T) {
	l := NewAuthCodeListener("/callback")
	port, err := l.Start(0)
	if err != nil {
		t.Fatalf("start error: %v", err)
	}
	defer l.Close()

	go func() {
		time.Sleep(20 * time.Millisecond)
		_, _ = http.Get(fmt.Sprintf("http://localhost:%d/callback?error=access_denied&error_description=User+denied", port))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err = l.WaitForCode(ctx, "some-state")
	if err == nil {
		t.Error("expected error for error callback, got nil")
	}
}

func TestAuthCodeListener_WaitForCode_ContextCancelled(t *testing.T) {
	l := NewAuthCodeListener("/callback")
	_, err := l.Start(0)
	if err != nil {
		t.Fatalf("start error: %v", err)
	}
	defer l.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = l.WaitForCode(ctx, "state")
	if err == nil {
		t.Error("expected context error, got nil")
	}
}

func TestAuthCodeListener_Close_Idempotent(t *testing.T) {
	l := NewAuthCodeListener("/callback")
	_, _ = l.Start(0)
	l.Close()
	if err := l.Close(); err != nil {
		t.Errorf("second Close() should not error: %v", err)
	}
}

// ─── OAuthClient.BuildAuthURL ─────────────────────────────────────────────────

func TestBuildAuthURL_Default(t *testing.T) {
	cfg := DefaultOAuthConfig()
	c := NewClient(cfg, nil, nil)
	authURL := c.BuildAuthURL(AuthURLParams{
		CodeChallenge: "challenge",
		State:         "state",
		Port:          8080,
	})
	if !strings.Contains(authURL, "console.anthropic.com") {
		t.Errorf("expected console URL, got %q", authURL)
	}
	if !strings.Contains(authURL, "code_challenge=challenge") {
		t.Errorf("expected code_challenge in URL: %q", authURL)
	}
	if !strings.Contains(authURL, "state=state") {
		t.Errorf("expected state in URL: %q", authURL)
	}
}

func TestBuildAuthURL_LoginWithClaude(t *testing.T) {
	cfg := DefaultOAuthConfig()
	c := NewClient(cfg, nil, nil)
	authURL := c.BuildAuthURL(AuthURLParams{
		CodeChallenge:   "ch",
		State:           "st",
		Port:            8080,
		LoginWithClaude: true,
	})
	if !strings.Contains(authURL, "claude.ai") {
		t.Errorf("expected claude.ai URL, got %q", authURL)
	}
}

func TestBuildAuthURL_InferenceOnly(t *testing.T) {
	cfg := DefaultOAuthConfig()
	c := NewClient(cfg, nil, nil)
	authURL := c.BuildAuthURL(AuthURLParams{
		CodeChallenge: "ch",
		State:         "st",
		Port:          8080,
		InferenceOnly: true,
	})
	if !strings.Contains(authURL, "scope=org%3Ainference") && !strings.Contains(authURL, "scope=org:inference") {
		t.Errorf("expected org:inference scope, got %q", authURL)
	}
}

// ─── OAuthClient.ExchangeCodeForTokens / RefreshToken ────────────────────────

func TestExchangeCodeForTokens_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := OAuthTokenExchangeResponse{
			AccessToken:  "acc-tok",
			RefreshToken: "ref-tok",
			ExpiresIn:    3600,
			Scope:        "org:inference user:profile",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := DefaultOAuthConfig()
	cfg.TokenURL = server.URL
	c := NewClient(cfg, nil, server.Client())

	tokens, err := c.ExchangeCodeForTokens(context.Background(), "code", "state", "verifier", 8080)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens.AccessToken != "acc-tok" {
		t.Errorf("expected acc-tok, got %q", tokens.AccessToken)
	}
	if len(tokens.Scopes) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(tokens.Scopes))
	}
}

func TestExchangeCodeForTokens_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
	}))
	defer server.Close()

	cfg := DefaultOAuthConfig()
	cfg.TokenURL = server.URL
	c := NewClient(cfg, nil, server.Client())

	_, err := c.ExchangeCodeForTokens(context.Background(), "code", "state", "verifier", 8080)
	if err == nil {
		t.Error("expected error for HTTP 400")
	}
}

func TestRefreshToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := OAuthTokenExchangeResponse{
			AccessToken: "refreshed-tok",
			ExpiresIn:   3600,
			Scope:       "org:inference",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := DefaultOAuthConfig()
	cfg.TokenURL = server.URL
	c := NewClient(cfg, nil, server.Client())

	tokens, err := c.RefreshToken(context.Background(), "old-refresh-tok", []string{"org:inference"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens.AccessToken != "refreshed-tok" {
		t.Errorf("expected refreshed-tok, got %q", tokens.AccessToken)
	}
	// When server doesn't return refresh_token, old one should be preserved
	if tokens.RefreshToken != "old-refresh-tok" {
		t.Errorf("expected old refresh token preserved, got %q", tokens.RefreshToken)
	}
}
