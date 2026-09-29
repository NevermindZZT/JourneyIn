package main

import "testing"

func TestIsLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8080", "localhost:8080", "[::1]:8080"} {
		if !isLoopback(addr) {
			t.Errorf("isLoopback(%q)=false, want true", addr)
		}
	}
	for _, addr := range []string{"0.0.0.0:8080", "journeyin.example:8080", "not-an-address"} {
		if isLoopback(addr) {
			t.Errorf("isLoopback(%q)=true, want false", addr)
		}
	}
}
