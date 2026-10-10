package main

import "testing"

func TestValidateAddressValid(t *testing.T) {
	valid := []string{
		"localhost:8080",
		"example.com:443",
		"192.168.1.10:22",
		"[::1]:80",
		"[2001:db8::1]:443",
		"host-name.internal:65535",
		"a:1",
	}
	for _, addr := range valid {
		if err := validateAddress(addr); err != nil {
			t.Errorf("validateAddress(%q) returned error: %v", addr, err)
		}
	}
}

func TestValidateAddressInvalid(t *testing.T) {
	invalid := []string{
		"",
		"   ",
		"localhost",
		"localhost:",
		":8080",
		"localhost:0",
		"localhost:65536",
		"localhost:http",
		"localhost:-1",
		"localhost:+80",
		"http://localhost:80",
		"local host:80",
		"::1:80",
		"[::1]",
		"host:80/path",
		"host:80:90",
	}
	for _, addr := range invalid {
		if err := validateAddress(addr); err == nil {
			t.Errorf("validateAddress(%q) returned nil, want error", addr)
		}
	}
}
