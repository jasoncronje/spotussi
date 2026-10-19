//go:build unix

package main

import (
	"os"
	"syscall"
)

// chownLikeDir gives files the owner of dir when running as root, so a
// restore run as root (to read backups owned by the host user) leaves
// files the server's non-root user can open.
func chownLikeDir(dir string, files ...string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	for _, f := range files {
		if err := os.Chown(f, int(st.Uid), int(st.Gid)); err != nil {
			return err
		}
	}
	return nil
}
