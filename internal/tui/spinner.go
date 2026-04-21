package tui

import "time"

// SpinnerMode controls how the spinner verb is displayed.
type SpinnerMode int

const (
	SpinnerModeNormal  SpinnerMode = iota // single agent
	SpinnerModeBrief                      // brief / compact
	SpinnerModeTeammate                   // multi-agent
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// SpinnerModel represents the loading spinner.
type SpinnerModel struct {
	frames  []string
	current int
	verb    string
	elapsed time.Duration
}

// newSpinner creates a default spinner.
func newSpinner() SpinnerModel {
	return SpinnerModel{
		frames: spinnerFrames,
		verb:   "Thinking",
	}
}

// Tick advances the spinner frame and accumulates elapsed time.
func (s SpinnerModel) Tick(d time.Duration) SpinnerModel {
	s.current = (s.current + 1) % len(s.frames)
	s.elapsed += d
	return s
}

// Reset resets the spinner to the initial state.
func (s SpinnerModel) Reset() SpinnerModel {
	s.current = 0
	s.elapsed = 0
	return s
}

// View renders the spinner line.
func (s SpinnerModel) View(theme Theme) string {
	frame := s.frames[s.current%len(s.frames)]
	return accentStyle(theme).Render(frame) + " " + s.verb + "… " +
		mutedStyle(theme).Render(formatDuration(s.elapsed))
}

// formatDuration returns a human-readable elapsed time string.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	secs := int(d.Seconds())
	if secs < 60 {
		return formatInt(secs) + "s"
	}
	mins := secs / 60
	secs = secs % 60
	return formatInt(mins) + "m" + formatInt(secs) + "s"
}

func formatInt(n int) string {
	return itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
