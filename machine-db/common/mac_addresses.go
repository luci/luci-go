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

// MAC48 represents a MAC-48 address.
type MAC48 uint64

// MaxMAC48 is the largest possible MAC-48 address.
const MaxMAC48 = MAC48(1<<48 - 1)

// String returns a string representation of this MAC-48 address.
func (m MAC48) String() string {
	mac := make(net.HardwareAddr, 8)
	binary.BigEndian.PutUint64(mac, uint64(m))
	return mac[2:].String()
}

// ParseMAC48 returns a MAC-48 address from the given string.
func ParseMAC48(mac48 string) (MAC48, error) {
	m, err := net.ParseMAC(mac48)
	if err != nil || len(m) != 6 {
		return 0, errors.Reason("invalid MAC-48 address %q", mac48).Err()
	}
	bytes := make([]byte, 8)
	copy(bytes[2:], m)
	return MAC48(binary.BigEndian.Uint64(bytes)), nil
}
