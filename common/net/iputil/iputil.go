// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package iputil is a collection of utilities for manipulating IP addresses that
// has been extracted from UFS because it is not UFS-specific.
package iputil

import (
	"encoding/binary"
	"net"

	"go.chromium.org/luci/common/errors"
)

// IPv4StrToInt returns an uint32 address from the given ipv4 address string.
//
// IPv6 addresses or non-parseable strings will return an error.
func IPv4StrToInt(ipAddress string) (uint32, error) {
	ip := net.ParseIP(ipAddress)
	if ip != nil {
		ip = ip.To4()
	}
	if ip == nil {
		return 0, errors.Fmt("invalid IPv4 address %q", ipAddress)
	}
	return binary.BigEndian.Uint32(ip), nil
}

// IPv4IntToStr returns a string ipv4 address
//
// Ipv6 addresses, or non-parseable strings will return an error.
func IPv4IntToStr(ipAddress uint32) string {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, ipAddress)
	return ip.String()
}
