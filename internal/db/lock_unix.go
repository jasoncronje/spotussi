//go:build unix

package db

import (
	"errors"
	"os"
	"syscall"
)

// lockFile takes an advisory lock on path+".lock": shared for every
// process that opens the database (server, CLI import), exclusive for a
// restore, which must not run while anything has the database open. The
// lock works across containers sharing the volume (same kernel).
func lockFile(path string, exclusive bool) (*os.File, error) {
	f, err := os.OpenFile(path+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrInUse
		}
		return nil, err
	}
	return f, nil
}
