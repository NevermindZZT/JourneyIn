//go:build !linux

package main

// Docker images are Linux-only. Other platforms keep their existing process
// identity and do not attempt to change filesystem ownership.
func prepareDockerRuntime(string) error { return nil }
