//go:build !windows

package main

// enableVT is only needed on Windows; other terminals understand escape codes already.
func enableVT() {}
