// Package browser provides a small, test-injectable wrapper for opening URLs
// in the user's default browser. Production code should call Open; tests may
// swap OpenFunc to capture the URL without actually launching anything.
package browser

import (
	"os/exec"
	"runtime"
)

// OpenFunc opens the given URL in the user's default browser. It is a package
// variable so tests can swap it for a stub. Production code uses openDefault.
var OpenFunc = openDefault

// Open is a convenience wrapper around OpenFunc.
func Open(url string) error {
	return OpenFunc(url)
}

func openDefault(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return nil
	}
	return cmd.Start()
}
