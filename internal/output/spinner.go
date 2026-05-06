package output

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// IsTerminal reports whether f points to a character-device terminal. Used
// to decide whether ANSI escape codes are safe to emit.
func IsTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// Spinner draws an animated, single-line progress indicator on a TTY. When
// the destination is not a TTY (piped output, MCP, CI, file redirect),
// every method is a no-op so machine-readable streams stay clean.
//
// Usage:
//
//	sp := NewSpinner(os.Stderr, 3*time.Minute)
//	sp.SetLabel("Waiting for credentials")
//	sp.Start(ctx)
//	defer sp.Stop()
//	// ... do work, optionally sp.SetLabel("Polling") later ...
type Spinner struct {
	f       *os.File
	enabled bool
	timeout time.Duration

	label   atomic.Value // string
	startAt time.Time

	stopOnce sync.Once
	done     chan struct{}
	wg       sync.WaitGroup
}

// NewSpinner returns a Spinner that writes to f. timeout is shown in the
// elapsed/total readout (e.g. "0:12 / 3:00") — pass time.Duration(0) to omit.
func NewSpinner(f *os.File, timeout time.Duration) *Spinner {
	s := &Spinner{
		f:       f,
		enabled: IsTerminal(f),
		timeout: timeout,
		done:    make(chan struct{}),
	}
	s.label.Store("")
	return s
}

// SetLabel updates the text shown next to the spinner. Safe to call from any
// goroutine, including before Start.
func (s *Spinner) SetLabel(label string) {
	s.label.Store(label)
}

// Start kicks off the animation goroutine. Calling Start more than once on
// the same Spinner is a no-op. The goroutine exits when ctx is canceled or
// Stop is called.
func (s *Spinner) Start(ctx context.Context) {
	if !s.enabled {
		return
	}
	s.startAt = time.Now()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.loop(ctx)
	}()
}

// Stop halts the spinner and clears the line. Safe to call multiple times.
func (s *Spinner) Stop() {
	if !s.enabled {
		return
	}
	s.stopOnce.Do(func() {
		close(s.done)
	})
	s.wg.Wait()
	fmt.Fprint(s.f, "\r\033[K")
}

func (s *Spinner) loop(ctx context.Context) {
	frames := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	i := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-ticker.C:
			s.draw(frames[i%len(frames)])
			i++
		}
	}
}

func (s *Spinner) draw(frame rune) {
	label, _ := s.label.Load().(string)
	elapsed := time.Since(s.startAt)
	if s.timeout > 0 {
		fmt.Fprintf(s.f, "\r\033[K%c %s   %s / %s", frame, label, formatMMSS(elapsed), formatMMSS(s.timeout))
	} else {
		fmt.Fprintf(s.f, "\r\033[K%c %s   %s", frame, label, formatMMSS(elapsed))
	}
}

func formatMMSS(d time.Duration) string {
	secs := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}
