//go:build vivahead

// Builds with -tags vivahead carry the headless driver (just test-ui); the release binary
// contains no test transport.
package main

import (
	_ "github.com/vivaui/viva/headless/auto"
)
