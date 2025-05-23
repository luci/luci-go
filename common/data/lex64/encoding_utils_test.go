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
	"bytes"
	"encoding/base64"
	"testing"
	"testing/quick"

	"github.com/google/go-cmp/cmp"
)

// TestDoEncodeDoDecode tests writing a message, encoding it, and then decoding it.
func TestDoEncodeDoDecode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		encoding *base64.Encoding
		in       []byte
		out      []byte
		ok       bool
	}{
		{
			name:     "empty string",
			encoding: base64.StdEncoding,
			in:       []byte{},
			out:      nil,
			ok:       true,
		},
		{
			name:     "simple string",
			encoding: base64.StdEncoding,
			in:       []byte("a\n"),
			out:      []byte("YQo="),
			ok:       true,
		},
		{
			name:     "bigger string",
			encoding: base64.StdEncoding,
			in:       []byte("c38541d2-0a5d-4341-bba6-df2b38835bc9\n"),
			out:      []byte("YzM4NTQxZDItMGE1ZC00MzQxLWJiYTYtZGYyYjM4ODM1YmM5Cg=="),
			ok:       true,
		},
		{
			name:     "sample string",
			encoding: base64.StdEncoding,
			in:       []byte{0xf1, 0xc5, 0x60, 0xbe, 0x24, 0x7e, 0xc3},
			out:      []byte("8cVgviR+ww=="),
			ok:       true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out, err := doEncode(tt.encoding, tt.in)
			switch {
			case err == nil && !tt.ok:
				t.Errorf("error is unexpectedly nil")
			case err != nil && tt.ok:
				t.Errorf("unexpected error: %s", err)
			}
			if diff := cmp.Diff(tt.out, out); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
			roundTrip, err := doDecode(tt.encoding, out)
			switch {
			case err == nil && !tt.ok:
				t.Errorf("error is unexpectedly nil")
			case err != nil && tt.ok:
				t.Errorf("unexpected error: %s", err)
			}
			if diff := cmp.Diff(tt.in, roundTrip); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

// TestRoundTrip tests that arbitrary messages can be encoded and decoded.
func TestRoundTrip(t *testing.T) {
	t.Parallel()

	doesRoundTrip := func(input []byte) bool {
		encoded, err := doEncode(base64.StdEncoding, input)
		if err != nil {
			panic(err)
		}
		roundTripped, err := doDecode(base64.StdEncoding, encoded)
		if err != nil {
			panic(err)
		}
		return bytes.Equal(input, roundTripped)
	}

	if err := quick.Check(doesRoundTrip, nil); err != nil {
		t.Errorf("unexpected error: %s", err)
	}
}
