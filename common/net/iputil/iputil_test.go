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

package iputil

import (
	"testing"
	"testing/quick"

	"github.com/google/go-cmp/cmp"
)

// TestIPv4StrToInt tests IPv4StrToInt on a few specific examples.
func TestIPv4StrToInt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  uint32
		ok    bool
	}{
		{
			name:  "zero",
			input: "0.0.0.0",
			want:  0x00_00_00_00,
			ok:    true,
		},
		{
			name:  "simple address",
			input: "1.2.3.4",
			want:  0x01_02_03_04,
			ok:    true,
		},
		{
			name:  "not an address",
			input: "aaaaaaaaa",
			want:  0,
			ok:    false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := IPv4StrToInt(tt.input)
			switch {
			case tt.ok && err != nil:
				t.Error(err)
			case !tt.ok && err == nil:
				t.Error("error is unexpectedly nil")
			}

			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("(-want +got): %s", diff)
			}
		})
	}
}

// TestIPv4RoundTrip tests that converting to and from an IPv4 using the helper functions round trips.
func TestIPv4RoundTrip(t *testing.T) {
	t.Parallel()

	roundTrip := func(ip uint32) bool {
		ipStr := IPv4IntToStr(ip)
		roundTripped, err := IPv4StrToInt(ipStr)
		if err != nil {
			return false
		}
		if ip != roundTripped {
			return false
		}
		return true
	}

	if err := quick.Check(roundTrip, nil); err != nil {
		t.Error(err)
	}
}
