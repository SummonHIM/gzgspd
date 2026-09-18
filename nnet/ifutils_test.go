package nnet

import (
	"testing"

	"github.com/summonhim/gzgspd/config"
)

func TestGetKeyIfName(t *testing.T) {
	if got := GetKeyIfName(config.ConfigInstance{}); got != "Auto" {
		t.Fatalf("empty interface should yield Auto, got %q", got)
	}
	if got := GetKeyIfName(config.ConfigInstance{Interface: "wanmac0"}); got != "wanmac0" {
		t.Fatalf("expected wanmac0, got %q", got)
	}
}

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
