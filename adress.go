package main

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

func validateAddress(s string) error {
	if strings.TrimSpace(s) == "" {
		return errors.New("address is empty")
	}
	if strings.ContainsAny(s, " \t/") {
		return errors.New("must be host:port without spaces, slashes or a URL scheme")
	}
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		var addrErr *net.AddrError
		if errors.As(err, &addrErr) {
			return fmt.Errorf("expected host:port: %s", addrErr.Err)
		}
		return fmt.Errorf("expected host:port: %v", err)
	}
	if host == "" {
		return errors.New("host is empty")
	}
	n, err := strconv.ParseUint(port, 10, 32)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("port %q is not a number between 1 and 65535", port)
	}
	return nil
}
