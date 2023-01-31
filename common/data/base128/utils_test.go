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
	"fmt"
	"testing"
	"testing/quick"

	"github.com/google/go-cmp/cmp"
)

// TestDecodedLen tests that the decoded length of an encoded string is correct
// in a few hand-verified cases.
func TestDecodedLen(t *testing.T) {
	t.Parallel()

	cases := []struct {
		dataLen    int
		encodedLen int
	}{
		{
			dataLen:    -1,
			encodedLen: -1,
		},
		{
			dataLen:    0,
			encodedLen: 0,
		},
		{
			dataLen:    1,
			encodedLen: 2,
		},
	}

	for _, tt := range cases {
		t.Run(fmt.Sprintf("%d --> %d", tt.dataLen, tt.encodedLen), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(tt.encodedLen, EncodedLen(tt.dataLen)); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
			if diff := cmp.Diff(tt.dataLen, DecodedLen(tt.encodedLen)); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

// TestDecodedLenRoundTrip is a property-based test that tests that taking a message of
// a given length, computing its encoded length, and then computing its decoded length,
// round-trips.
//
// This round-trip property should hold even in cases where the length is invalid
// (congruent to 1 mod 8 or negative).
func TestDecodedLenRoundTrip(t *testing.T) {
	t.Parallel()

	// Choose bounds to avoid annoying wraparound behvaior.
	// If you use a string that big, then all bets are off.
	const maxInt = 1 << 60
	const minInt = -1 * (1 << 30)

	roundTrip := func(input int) bool {
		if input > maxInt {
			return true
		}
		if input < minInt {
			return true
		}
		return input == DecodedLen(EncodedLen(input))
	}

	if err := quick.Check(roundTrip, nil); err != nil {
		t.Error(err)
	}
}
