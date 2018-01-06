// Copyright 2017 The LUCI Authors.
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

// Returns the starting address and length of the given IPv4 CIDR block.
func IPv4Range(block string) (uint32, int64, error) {
	ip, subnet, err := net.ParseCIDR(block)
	if err != nil {
		return 0, 0, errors.Reason("invalid CIDR block %q", block).Err()
	}
	ipv4 := ip.Mask(subnet.Mask)
	if len(ipv4) != 4 {
		return 0, 0, errors.Reason("invalid IPv4 CIDR block %q", block).Err()
	}
	ones, _ := subnet.Mask.Size()
	return binary.BigEndian.Uint32(ipv4), 1 << uint32(32 - ones), nil
}

// Parses an integer IPv4 address.
func ParseIPv4(ipv4 uint32) net.IP {
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, ipv4)
	return net.IPv4(bytes[0], bytes[1], bytes[2], bytes[3])
}
