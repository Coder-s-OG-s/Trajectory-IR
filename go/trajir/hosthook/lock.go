package hosthook

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
)

// lockWait is how long a hook waits for another hook on the same
// trajectory. lockStale is the age past which a lock file counts as left
// behind by a dead process. Both match integrations/hosthook/session.py.
var (
	lockWait   = 15 * time.Second
	lockStale  = 15 * time.Second
	lockPoll   = 50 * time.Millisecond
	lockDenied = time.Second
)

// acquireLock takes a create-exclusive lock file. It works across
// processes, which is what matters: each host hook is its own process.
func acquireLock(path string, wait time.Duration) (func(), error) {
	deadline := time.Now().Add(wait)
	var deniedSince time.Time
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err == nil {
			return func() {
				_ = f.Close()
				_ = os.Remove(path)
			}, nil
		}
		if errors.Is(err, fs.ErrPermission) {
			// Windows answers "access denied", not "exists", while another
			// hook's lock file is being deleted. That clears within
			// milliseconds. A denial that lasts is a real permission problem.
			if deniedSince.IsZero() {
				deniedSince = time.Now()
			}
			if time.Since(deniedSince) > lockDenied || time.Now().After(deadline) {
				return nil, err
			}
			time.Sleep(lockPoll)
			continue
		}
		deniedSince = time.Time{}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > lockStale {
			// Windows refuses to delete a lock another process still has
			// open. Then fall through to the sleep and honor the deadline.
			if os.Remove(path) == nil {
				continue
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("trajir hook lock held: %s", path)
		}
		time.Sleep(lockPoll)
	}
}
