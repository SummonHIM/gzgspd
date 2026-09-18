package nnet

import (
	"testing"
)

func TestGetIPMACInvalid(t *testing.T) {
	if _, err := GetIPMAC("not-an-ip"); err == nil {
		t.Fatal("expected error for invalid ip")
	}
	if _, err := GetIPMAC("::1"); err == nil {
		t.Fatal("expected error for ipv6 address")
	}
}

func TestGetIfMACMissing(t *testing.T) {
	if _, err := GetIfMAC("definitely-not-an-interface-xyz"); err == nil {
		t.Fatal("expected error for missing interface")
	}
}
