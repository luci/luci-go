// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// parseRemoteIP parses the IP address from the supplied HTTP request RemoteAddr
// string.
//
// The HTTP library's representation of remote address says that it is a
// "host:port" pair. They lie. The following permutations have been observed:
// - ipv4:port (Standard)
// - ipv4 (GAE)
// - ipv6 (GAE)
// - [ipv6]:port (GAE Managed VM)
//
// Note that IPv6 addresses have colons in them.
//
// If the remote address is blank, IPv6 loopback (::1) will be returned.
func parseRemoteIP(a string) (net.IP, error) {
	if a == "" {
		return net.IPv6loopback, nil
	}

	// IPv6 in braces with or without port.
	if strings.HasPrefix(a, "[") {
		idx := strings.LastIndex(a, "]")
		if idx < 0 {
			return nil, errors.New("missing closing brace on IPv6 address")
		}
		a = a[1:idx]
	}

	// Parse as a standalone IPv4/IPv6 address (no port).
	if ip := net.ParseIP(a); ip != nil {
		return ip, nil
	}

	// Is there a port? Strip and try again.
	if idx := strings.LastIndex(a, ":"); idx >= 0 {
		if ip := net.ParseIP(a[:idx]); ip != nil {
			return ip, nil
		}
	}

	return nil, fmt.Errorf("don't know how to parse: %q", a)
}
