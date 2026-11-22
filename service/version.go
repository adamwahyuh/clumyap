package main

import (
	_ "embed"
	"strings"
)

// VERSION is the single source of the app version; it is compiled into the binary.
//
//go:embed VERSION
var versionFile string

func version() string { return strings.TrimSpace(versionFile) }
