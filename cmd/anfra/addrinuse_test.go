package main

import (
	"net"
	"testing"
)

// A taken address is told apart from other failures on every OS: serve moves
// aside only for it.
func TestIsAddrInUse(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	_, err = net.Listen("tcp", taken.Addr().String())
	if !isAddrInUse(err) {
		t.Errorf("isAddrInUse(%v) = false, want true", err)
	}
	if _, err := net.Listen("tcp", "127.0.0.1:99999"); isAddrInUse(err) {
		t.Errorf("isAddrInUse(%v) = true, want false", err)
	}
}
