//go:build !unix

package main

func chownLikeDir(string, ...string) error { return nil }
