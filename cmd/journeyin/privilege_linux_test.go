//go:build linux

package main

import "testing"

func TestDockerPathWithin(t *testing.T) {
	tests := map[string]bool{
		"/data":               true,
		"/data/journeyin.db":  true,
		"/data/nested/db":     true,
		"/data/../etc":        false,
		"/data2/journeyin.db": false,
		"/var/lib/journeyin":  false,
	}
	for path, want := range tests {
		if got := dockerPathWithin(path); got != want {
			t.Errorf("dockerPathWithin(%q) = %v, want %v", path, got, want)
		}
	}
}
