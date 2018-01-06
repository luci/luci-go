// Copyright 2018 The LUCI Authors.
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

// Package common contains common Machine Database functions.
package common

import (
	"encoding/binary"
	"net"

	"go.chromium.org/luci/common/errors"
)

// IPv4Range returns the starting address and length of the given IPv4 CIDR block.
func IPv4Range(block string) (uint32, int64, error) {
	ip, subnet, err := net.ParseCIDR(block)
	if err != nil {
		return 0, 0, errors.Reason("invalid CIDR block %q", block).Err()
	}
	ipv4, err := IPv4ToUint32(ip.Mask(subnet.Mask))
	if err != nil {
		return 0, 0, errors.Reason("invalid IPv4 CIDR block").Err()
	}
	ones, _ := subnet.Mask.Size()
	return ipv4, 1 << uint32(32-ones), nil
}

// IPv4ToUint32 returns a uint32 representation of the given net.IP.
func IPv4ToUint32(ip net.IP) (uint32, error) {
	ipv4 := ip.To4()
	if ipv4 == nil {
		return 0, errors.Reason("invalid IPv4 address %q", ip).Err()
	}
	return binary.BigEndian.Uint32(ipv4), nil
}

// Uint32ToIPv4 returns a net.IP address from the given uint32.
func Uint32ToIPv4(ipv4 uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, ipv4)
	return ip
}
