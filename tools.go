//go:build tools
// +build tools

// Official workaround to track tool dependencies with go modules:
// https://go.dev/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module

// TODO: with go 1.24 we should be able to use the `go tool` command
// https://tip.golang.org/doc/go1.24#go-command

package tools

import (
	// used to generate mocks
	_ "go.uber.org/mock/mockgen"
)
