//go:build windows

package robustio

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// giveUpAfter is how long Rename retries: a reader holds a file for moments,
// another program (an antivirus scan, an editor) may hold it for good.
const giveUpAfter = time.Second

// Rename is os.Rename, retried for a moment while another process has newpath
// open: Windows refuses to rename over a file that is open, as anfra's own
// readers briefly keep it, where Unix allows it.
func Rename(oldpath, newpath string) error {
	deadline := time.Now().Add(giveUpAfter)
	wait := time.Millisecond
	for {
		err := os.Rename(oldpath, newpath)
		if err == nil || !inUse(err) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w (another program may have %s open: close it, and try again)", err, newpath)
		}
		time.Sleep(wait)
		wait = min(2*wait, 100*time.Millisecond)
	}
}

// inUse reports whether err is Windows refusing a file another process has
// open. Access denied is also a lasting refusal (a read-only file); retrying it
// for a moment costs only that moment.
func inUse(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
