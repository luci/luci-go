// Copyright 2022 The LUCI Authors.
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

package lex64

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"testing/quick"

	"github.com/google/go-cmp/cmp"
)

// TestAlphabet checks that the alphabet is in lexicographic order.
func TestAlphabet(t *testing.T) {
	t.Parallel()

	as := []string{
		alphabetV1,
		alphabetV2,
		fmt.Sprintf("-%s", alphabetV1),
		fmt.Sprintf("-%s", alphabetV2),
	}

	for _, alphabet := range as {
		sortedAlphabet := sortString(alphabet)

		if diff := cmp.Diff(sortedAlphabet, alphabet); diff != "" {
			t.Errorf("unexpected diff (-want +got): %s", diff)
		}
	}
}

// TestGetEncoding tests that invalid encoding names produce an error with
// the expected message.
func TestGetEncoding(t *testing.T) {
	t.Parallel()

	_, err := GetEncoding(Scheme("17b72b62-202c-4e09-91f1-94bbd1b2ede0"))
	switch err {
	case nil:
		t.Error("error unexpectedly nil")
	default:
		expected := fmt.Sprintf("invalid lex64 version %q", "17b72b62-202c-4e09-91f1-94bbd1b2ede0")
		actual := err.Error()
		if diff := cmp.Diff(expected, actual); diff != "" {
			t.Errorf("unexpected diff (-want +got): %s", diff)
		}
	}
}

// TestEncodeAndDecode tests that encoding and decoding work and roundtrip as
// expected.
func TestEncodeAndDecode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		in       string
		encoding Scheme
		out      string
	}{
		{
			name:     "F1F2F3 v1",
			in:       "\xF1\xF2\xF3",
			encoding: V1,
			out:      "wUAn",
		},
		{
			name:     "F1F2F3 v2",
			in:       "\xF1\xF2\xF3",
			encoding: V2,
			out:      "wUAn",
		},
		{
			name:     "empty string v1",
			in:       "",
			encoding: V1,
			out:      "",
		},
		{
			name:     "empty string v2",
			in:       "",
			encoding: V1,
			out:      "",
		},
		{
			name:     "single char v1",
			in:       "\x00",
			encoding: V1,
			out:      "00",
		},
		{
			name:     "single char v2",
			in:       "\x00",
			encoding: V2,
			out:      "..",
		},
		{
			name:     "random string v1",
			in:       "\x67\x9a\x5c\x48\xbe\x97\x27\x75\xdf\x6a",
			encoding: V1,
			out:      "OtdRHAuM9rMUPV",
		},
		{
			name:     "random string v2",
			in:       "\x67\x9a\x5c\x48\xbe\x97\x27\x75\xdf\x6a",
			encoding: V2,
			out:      "OtdRHAuM8rMUPV",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			encoding := encodings[tt.encoding]

			in := []byte(tt.in)
			actual, err := Encode(encoding, in)
			if err != nil {
				t.Errorf("unexpected error for subtest %q: %s", tt.name, err)
			}
			expected := tt.out

			if diff := cmp.Diff(expected, actual); diff != "" {
				t.Errorf("unexpected diff during encoding for subtest %q (-want +got): %s", tt.name, diff)
			}
		})
	}
}

// TestRoundTripEquality tests that encoding and decoding with padding
// round-trips.
func TestRoundTripEquality(t *testing.T) {
	t.Parallel()

	for _, v := range []struct {
		opts Scheme
	}{
		{
			opts: V1,
		},
		{
			opts: V1Padding,
		},
		{
			opts: V2,
		},
		{
			opts: V2Padding,
		},
	} {
		roundTrip := func(a []byte) bool {
			encoding, _ := GetEncoding(v.opts)
			encoded, err := Encode(encoding, a)
			if err != nil {
				panic(err.Error())
			}
			decoded, err := Decode(encoding, encoded)
			if err != nil {
				panic(err.Error())
			}
			return cmpBytes(a, decoded) == 0
		}

		if err := quick.Check(roundTrip, nil); err != nil {
			t.Errorf("unexpected error: %s", err)
		}
	}
}

// TestPreservationOfComparisonOrder tests that encoding with padding preserves
// comparison order.
func TestPreservationOfComparisonOrder(t *testing.T) {
	t.Parallel()

	for _, v := range []struct {
		name string
		opts Scheme
	}{
		{
			name: "current",
			opts: V2,
		},
		{
			name: "current with padding",
			opts: V2Padding,
		},
		{
			name: "legacy",
			opts: V1,
		},
		{
			name: "legacy with padding",
			opts: V1Padding,
		},
	} {
		t.Run(v.name, func(t *testing.T) {
			t.Parallel()
			expected := func(a []byte, b []byte) int {
				return cmpBytes(a, b)
			}

			actual := func(a []byte, b []byte) int {
				encoding, _ := GetEncoding(v.opts)
				a1, err := Encode(encoding, a)
				if err != nil {
					panic(err.Error())
				}
				b1, err := Encode(encoding, b)
				if err != nil {
					panic(err.Error())
				}
				return strings.Compare(a1, b1)
			}
			if err := quick.CheckEqual(expected, actual, &quick.Config{
				MaxCount: 10000,
			}); err != nil {
				t.Errorf("unexpected error: %s", err)
			}
		})
	}
}

// TestConcatenationFails is a demonstration that concatenation is not respected.
func TestConcatenationFails(t *testing.T) {
	t.Parallel()

	encoding, _ := GetEncoding(V2Padding)

	a := []byte("a")
	b := []byte("b")

	a2, err := Encode(encoding, a)
	if err != nil {
		t.Error(err)
	}

	b2, err := Encode(encoding, b)
	if err != nil {
		t.Error(err)
	}

	_, err = Decode(encoding, fmt.Sprintf("%s%s", a2, b2))
	switch err {
	case nil:
		t.Error("error is unexpectedly nil")
	default:
		if !strings.Contains(err.Error(), "illegal base64") {
			t.Errorf("wrong error: %s", err)
		}
	}
}

// cmpString compares two sequences of bytes and returns +1, 0, or -1.
func cmpBytes(a []byte, b []byte) int {
	return strings.Compare(string(a), string(b))
}

// sortString sorts a string characterwise.
func sortString(s string) string {
	chars := strings.Split(s, "")
	sort.Strings(chars)
	return strings.Join(chars, "")
}
