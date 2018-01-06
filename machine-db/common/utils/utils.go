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

// Package utils contains Machine Database utility functions.
package utils

import (
	"encoding/binary"
	"math"
	"net"

	"go.chromium.org/luci/common/errors"
)

// Returns the starting IP address and number of IP addresses in the given IPv4 CIDR block.
func Range(block string) (uint32, int, error) {
	ip, subnet, err := net.ParseCIDR(block)
	if err != nil {
		return 0, 0, errors.Reason("invalid CIDR block %q", block).Err()
	}
	ipv4 := ip.Mask(subnet.Mask)
	if len(ipv4) != 4 {
		return 0, 0, errors.Reason("invalid IPv4 CIDR block %q", block).Err()
	}
	ones, _ := subnet.Mask.Size()
	return binary.BigEndian.Uint32(ip.Mask(subnet.Mask)), int(math.Exp2(float64(32 - ones))), nil
}
