package api_test

import (
	"crypto/rand"
	"testing"
)

// testAddress is a pipe of a name the test's own.
func testAddress(*testing.T) string { return `\\.\pipe\hayami-test-` + rand.Text() }
