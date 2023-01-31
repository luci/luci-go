// Copyright 2023 The LUCI Authors.
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

package base128

import (
	"encoding/hex"
	"fmt"
	"testing"
)

// TestValidateLength tests that lengths that are negative or
// congruent to 1 mod 8 are successfully rejected by ValidateLength.
func TestValidateLength(t *testing.T) {
	t.Parallel()

	cases := []struct {
		length int
		ok     bool
	}{
		{
			length: -1,
			ok:     false,
		},
		{
			length: 0,
			ok:     true,
		},
		{
			length: 1,
			ok:     false,
		},
		{
			length: 2,
			ok:     true,
		},
		{
			length: 8,
			ok:     true,
		},
		{
			length: 9,
			ok:     false,
		},
		{
			length: 10,
			ok:     true,
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(fmt.Sprintf("%d", tt.length), func(t *testing.T) {
			t.Parallel()
			err := ValidateLength(tt.length)
			switch {
			case err == nil && !tt.ok:
				t.Errorf("error was unexpectedly nil")
			case err != nil && tt.ok:
				t.Errorf("unexpected error: %s", err)
			}
		})
	}
}

// TestValidate tests that encoded strings with bytes with the leading 1-bit
// set are rejected.
func TestValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		msg []byte
		ok  bool
	}{
		{
			msg: []byte{},
			ok:  true,
		},
		{
			msg: []byte{0b_0000_0000},
			ok:  true,
		},
		{
			msg: []byte{0b_1000_0000},
			ok:  false,
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(hex.EncodeToString(tt.msg), func(t *testing.T) {
			t.Parallel()
			err := Validate(tt.msg)
			switch {
			case err == nil && !tt.ok:
				t.Errorf("error was unexpectedly nil")
			case err != nil && tt.ok:
				t.Errorf("unexpected error: %s", err)
			}
		})
	}
}
