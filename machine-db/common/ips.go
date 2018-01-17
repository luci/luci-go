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

// IPv4 represents an IPv4 address.
type IPv4 uint32

// String returns a string representation of this IPv4 address.
func (i IPv4) String() string {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, uint32(i))
	return ip.String()
}

// IPv4Range returns the starting address and length of the given IPv4 CIDR block.
func IPv4Range(block string) (IPv4, int64, error) {
	ip, subnet, err := net.ParseCIDR(block)
	if err != nil {
		return 0, 0, errors.Reason("invalid CIDR block %q", block).Err()
	}
	ipv4 := ip.Mask(subnet.Mask).To4()
	if ipv4 == nil {
		return 0, 0, errors.Reason("invalid IPv4 CIDR block %q", block).Err()
	}
	ones, _ := subnet.Mask.Size()
	return IPv4(binary.BigEndian.Uint32(ipv4)), 1 << uint32(32-ones), nil
}
