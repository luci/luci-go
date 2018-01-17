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

// MaxMAC48 is the largest possible MAC-48 address.
const MaxMAC48 = 255<<40 | 255<<32 | 255<<24 | 255<<16 | 255<<8 | 255

// MAC48ToUint64 returns a uint64 representation of the given net.HardwareAddr.
func MAC48ToUint64(mac48 net.HardwareAddr) (uint64, error) {
	if len(mac48) != 6 {
		return 0, errors.Reason("invalid MAC-48 address %q", mac48).Err()
	}
	bytes := make([]byte, 8)
	copy(bytes[2:], mac48)
	return binary.BigEndian.Uint64(bytes), nil
}

// MAC48StringToUint64 returns a uint64 representation of the given string.
func MAC48StringToUint64(mac48 string) (uint64, error) {
	mac, err := net.ParseMAC(mac48)
	if err != nil {
		return 0, errors.Reason("invalid MAC-48 address %q", mac48).Err()
	}
	return MAC48ToUint64(mac)
}

// Uint64ToMAC48 returns a net.HardwareAddr from the given uint64.
func Uint64ToMAC48(mac48 uint64) (net.HardwareAddr, error) {
	if mac48 > MaxMAC48 {
		return nil, errors.Reason("invalid MAC-48 address %d", mac48).Err()
	}
	bytes := make([]byte, 8)
	binary.BigEndian.PutUint64(bytes, mac48)
	mac := make(net.HardwareAddr, 6)
	copy(mac, bytes[2:])
	return mac, nil
}
