//go:build !windows

package main

import "os"

// removeInstallDir deletes the installed files; a running clumyap keeps working until it exits.
func removeInstallDir(dir string) error { return os.RemoveAll(dir) }
