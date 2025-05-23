// Copyright 2025 The LUCI Authors.
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

package sink

import (
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

func TestTruncateTestIDComponent(t *testing.T) {
	t.Parallel()
	ftt.Run(`truncateTestIDComponent`, t, func(t *ftt.Test) {
		testCases := []struct {
			name                  string
			input                 string
			maxLengthBytes        int
			additionalCutoutBytes int
			wantTruncated         bool
			wantResult            string
			wantResultBytes       int
		}{
			{
				name:                  "no truncation",
				input:                 "fooBar",
				maxLengthBytes:        10,
				additionalCutoutBytes: 0,
				wantTruncated:         false,
				wantResult:            "fooBar",
				wantResultBytes:       6,
			},
			{
				name:                  "no truncation with escapes",
				input:                 "fooBar!#:\\", // !,#,:,\ needs to be escaped, so uses two bytes each.
				maxLengthBytes:        14,
				additionalCutoutBytes: 0,
				wantTruncated:         false,
				wantResult:            "fooBar!#:\\",
				wantResultBytes:       14,
			},
			{
				name:                  "no truncation with cutout",
				input:                 "fooBar",
				maxLengthBytes:        6,
				additionalCutoutBytes: 3,
				wantTruncated:         false,
				wantResult:            "fooBar",
				wantResultBytes:       6,
			},
			{
				name:                  "no truncation with multi-byte rune",
				input:                 "fooBar€", // € is 0xE2 0x82 0xAC in UTF-8 (3 bytes).
				maxLengthBytes:        9,
				additionalCutoutBytes: 0,
				wantTruncated:         false,
				wantResult:            "fooBar€",
				wantResultBytes:       9,
			},
			{
				name:                  "truncation",
				input:                 "fooBar",
				maxLengthBytes:        3,
				additionalCutoutBytes: 0,
				wantTruncated:         true,
				wantResult:            "foo",
				wantResultBytes:       3,
			},
			{
				name:                  "truncation with cutout",
				input:                 "fooBar",
				maxLengthBytes:        5,
				additionalCutoutBytes: 2,
				wantTruncated:         true,
				wantResult:            "foo",
				wantResultBytes:       3,
			},
			{
				name:                  "truncation with escapes",
				input:                 "fooBar!#:\\", // !,#,:,\\ needs to be escaped, so uses two bytes each.
				maxLengthBytes:        13,
				additionalCutoutBytes: 0,
				wantTruncated:         true,
				wantResult:            "fooBar!#:",
				wantResultBytes:       12,
			},
			{
				name:                  "truncation with escapes and cutout",
				input:                 "fooBar!#:\\", // !,#,:,\\ needs to be escaped, so uses two bytes each.
				maxLengthBytes:        13,
				additionalCutoutBytes: 2,
				wantTruncated:         true,
				wantResult:            "fooBar!#",
				wantResultBytes:       10,
			},
			{
				name:                  "truncation with multi-byte rune",
				input:                 "fooBar€", // € is 0xE2 0x82 0xAC in UTF-8 (3 bytes).
				maxLengthBytes:        8,
				additionalCutoutBytes: 0,
				wantTruncated:         true,
				wantResult:            "fooBar",
				wantResultBytes:       6,
			},
			{
				name:                  "truncation with multi-byte rune and cutout",
				input:                 "fooBar€", // € is 0xE2 0x82 0xAC in UTF-8 (3 bytes).
				maxLengthBytes:        8,
				additionalCutoutBytes: 5,
				wantTruncated:         true,
				wantResult:            "foo",
				wantResultBytes:       3,
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *ftt.Test) {
				gotTruncated, gotResult, gotResultBytes := truncateTestIDComponent(tc.input, tc.maxLengthBytes, tc.additionalCutoutBytes)
				assert.That(t, gotTruncated, should.Equal(tc.wantTruncated))
				assert.That(t, gotResult, should.Equal(tc.wantResult))
				assert.That(t, gotResultBytes, should.Equal(tc.wantResultBytes))
			})
		}
	})
}

func TestShortenIDComponent(t *testing.T) {
	t.Parallel()
	ftt.Run(`shortenIDComponent`, t, func(t *ftt.Test) {
		testCases := []struct {
			name            string
			input           string
			maxLengthBytes  int
			wantResult      string
			wantResultBytes int
			wantErr         string
		}{
			{
				name:            "no shortening",
				input:           "fooBar01234567901234567",
				maxLengthBytes:  100,
				wantResult:      "fooBar01234567901234567",
				wantResultBytes: 6 + 17,
			},
			{
				name:            "no shortening with escapes",
				input:           "fooBar!#:\\01234567901234567", // !,#,:,\ needs to be escaped, so uses two bytes each.
				maxLengthBytes:  100,
				wantResult:      "fooBar!#:\\01234567901234567",
				wantResultBytes: 6 + 8 + 17,
			},
			{
				name:            "no shortening with multi-byte rune",
				input:           "fooBar€01234567901234567", // € is 0xE2 0x82 0xAC in UTF-8 (3 bytes).
				maxLengthBytes:  100,
				wantResult:      "fooBar€01234567901234567",
				wantResultBytes: 6 + 3 + 17,
			},
			{
				name:            "shortening",
				input:           "fooBar01234567901234567",
				maxLengthBytes:  20,
				wantResult:      "foo~04bcaf66aea4c0c9",
				wantResultBytes: 3 + 17,
			},
			{
				name:            "shortening with escapes",
				input:           "fooBar!#:\\01234567901234567", // !,#,:,\\ needs to be escaped, so uses two bytes each.
				maxLengthBytes:  26,
				wantResult:      "fooBar!~c77928ccaa3bf0e8",
				wantResultBytes: 6 + 2 + 17,
			},
			{
				name:            "shortening with multi-byte rune",
				input:           "fooBar€€01234567901234567", // € is 0xE2 0x82 0xAC in UTF-8 (3 bytes).
				maxLengthBytes:  28,
				wantResult:      "fooBar€~f522cec424fca5d2",
				wantResultBytes: 6 + 3 + 17,
			},
			{
				name:           "shortening with invalid UTF-8",
				input:          "fooBar\x80",
				maxLengthBytes: 20,
				wantErr:        "not a valid utf8 string",
			},
			{
				name:           "shortening with non-printable",
				input:          "fooBar\n",
				maxLengthBytes: 20,
				wantErr:        "non-printable rune '\\n' at byte index 6",
			},
			{
				name:           "shortening with unicode replacement character",
				input:          "fooBar\uFFFD",
				maxLengthBytes: 20,
				wantErr:        "unicode replacement character (U+FFFD) at byte index 6",
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *ftt.Test) {
				gotResult, gotResultBytes, err := shortenIDComponent(tc.input, tc.maxLengthBytes)
				if tc.wantErr != "" {
					assert.Loosely(t, err, should.ErrLike(tc.wantErr))
				} else {
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, gotResult, should.Equal(tc.wantResult))
					assert.That(t, gotResultBytes, should.Equal(tc.wantResultBytes))
				}
			})
		}
	})
}

func TestShortenCaseNameComponents(t *testing.T) {
	t.Parallel()
	ftt.Run(`shortenCaseNameComponents`, t, func(t *ftt.Test) {
		testCases := []struct {
			name            string
			input           []string
			maxLengthBytes  int
			wantResult      []string
			wantResultBytes int
			wantErr         string
		}{
			{
				name:            "no shortening, single component",
				input:           []string{strings.Repeat("a", 100)},
				maxLengthBytes:  100,
				wantResult:      []string{strings.Repeat("a", 100)},
				wantResultBytes: 100,
			},
			{
				name:            "no shortening, multi-component",
				input:           []string{"foo", "bar", "01234567901234567"},
				maxLengthBytes:  100,
				wantResult:      []string{"foo", "bar", "01234567901234567"},
				wantResultBytes: 3 + 1 + 3 + 1 + 17, // 25
			},
			{
				name:            "shortening, single component",
				input:           []string{strings.Repeat("a", 101)},
				maxLengthBytes:  100,
				wantResult:      []string{strings.Repeat("a", 100-17) + "~9d0793397991b57a"},
				wantResultBytes: 100,
			},
			{
				name:            "shortening, multi-component",
				input:           []string{"foo", "bariometric", "012345679012345"},
				maxLengthBytes:  32,
				wantResult:      []string{"foo", "bariometric~f6fa0a712fce34ff"},
				wantResultBytes: 3 + 1 + (11 + 17), // 32
			},
			{
				name:            "shortening, multi-component #2",
				input:           []string{"foo", "bariometric", "012345679012345"},
				maxLengthBytes:  25,
				wantResult:      []string{"foo", "bari~f6fa0a712fce34ff"},
				wantResultBytes: 3 + 1 + (4 + 17), // 25
			},
			{
				name:            "shortening, multi-component #3",
				input:           []string{"foo", "bariometric", "012345679012345"},
				maxLengthBytes:  24,
				wantResult:      []string{"foo~f6fa0a712fce34ff"},
				wantResultBytes: 3 + 17, // 20
			},
			{
				name:            "shortening with multi-byte rune",
				input:           []string{"foo", "bar€€", "0123456790123456789012345"}, // € is 0xE2 0x82 0xAC in UTF-8 (3 bytes).
				maxLengthBytes:  30,
				wantResult:      []string{"foo", "bar€€~0c09cbe22d3be392"},
				wantResultBytes: 3 + 1 + (3 + 3 + 20), // 27
			},
			{
				name:            "shortening with multi-byte rune #2",
				input:           []string{"foo", "€", "0123456790123456789012345"}, // € is 0xE2 0x82 0xAC in UTF-8 (3 bytes).
				maxLengthBytes:  33,
				wantResult:      []string{"foo", "€", "0123456790123456789012345"},
				wantResultBytes: 3 + 1 + 3 + 1 + 25, // 33
			},
			{
				name:            "shortening with multi-byte rune #3",
				input:           []string{"foo", "€", "0123456790123456789012345"}, // € is 0xE2 0x82 0xAC in UTF-8 (3 bytes).
				maxLengthBytes:  32,
				wantResult:      []string{"foo", "€", "0123456~eeada1052899aeaa"},
				wantResultBytes: 3 + 1 + 3 + 1 + (7 + 17), // 32
			},
			{
				name:            "shortening with multi-byte rune #4",
				input:           []string{"foo", "€", "0123456790123456789012345"}, // € is 0xE2 0x82 0xAC in UTF-8 (3 bytes).
				maxLengthBytes:  29,
				wantResult:      []string{"foo", "€", "0123~eeada1052899aeaa"},
				wantResultBytes: 3 + 1 + 3 + 1 + (4 + 17), // 29
			},
			{
				name:            "shortening with multi-byte rune #5",
				input:           []string{"foo", "€", "0123456790123456789012345"}, // € is 0xE2 0x82 0xAC in UTF-8 (3 bytes).
				maxLengthBytes:  28,
				wantResult:      []string{"foo", "€~eeada1052899aeaa"},
				wantResultBytes: 3 + 1 + (3 + 17), // 24
			},
			{
				name:            "shortening with multi-byte rune #6",
				input:           []string{"foo", "€", "0123456790123456789012345"}, // € is 0xE2 0x82 0xAC in UTF-8 (3 bytes).
				maxLengthBytes:  25,
				wantResult:      []string{"foo", "€~eeada1052899aeaa"},
				wantResultBytes: 3 + 1 + (3 + 17), // 24
			},
			{
				name:            "shortening with multi-byte rune #7",
				input:           []string{"foo", "€"}, // € is 0xE2 0x82 0xAC in UTF-8 (3 bytes).
				maxLengthBytes:  25,
				wantResult:      []string{"foo", "€"},
				wantResultBytes: 3 + 1 + 3, // 7
			},
			{
				name:            "shortening with salami sliced components",
				input:           []string{"foo", "a", "b", "c", "d", "e", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9"},
				maxLengthBytes:  27,
				wantResult:      []string{"foo", "a", "b~32fa2c2da4a0f854"},
				wantResultBytes: 3 + 1 + 1 + 1 + (1 + 17), // 24
			},
			{
				name:            "shortening with salami sliced components #2",
				input:           []string{"foo", "a", "b", "c", "d", "e", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9"},
				maxLengthBytes:  28,
				wantResult:      []string{"foo", "a", "b~32fa2c2da4a0f854"},
				wantResultBytes: 3 + 1 + 1 + 1 + (1 + 17), // 24
			},
			{
				name:            "shortening with salami sliced components #3",
				input:           []string{"foo", "a", "b", "c", "d", "e", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9"},
				maxLengthBytes:  29,
				wantResult:      []string{"foo", "a", "b", "c~32fa2c2da4a0f854"},
				wantResultBytes: 3 + 1 + 1 + 1 + 1 + 1 + (1 + 17), // 26
			},
			{
				name:            "shortening with salami sliced components and multi-byte rune",
				input:           []string{"foo", "a", "b", "€", "d", "e", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}, // € is 0xE2 0x82 0xAC in UTF-8 (3 bytes).
				maxLengthBytes:  29,
				wantResult:      []string{"foo", "a", "b", "€~ec97629837decc5f"},
				wantResultBytes: 3 + 1 + 1 + 1 + 1 + 1 + (3 + 17), // 28
			},
			{
				name:            "shortening with salami sliced components and multi-byte rune #2",
				input:           []string{"foo", "a", "b", "€ab", "d", "e", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}, // € is 0xE2 0x82 0xAC in UTF-8 (3 bytes).
				maxLengthBytes:  29,
				wantResult:      []string{"foo", "a", "b", "€a~dd68b631d67945ee"},
				wantResultBytes: 3 + 1 + 1 + 1 + 1 + 1 + (3 + 1 + 17), // 29
			},
			{
				name:           "shortening with invalid UTF-8",
				input:          []string{"foo", "bar\x80"},
				maxLengthBytes: 21,
				wantErr:        "component[1]: not a valid utf8 string",
			},
			{
				name:           "shortening with non-printable",
				input:          []string{"foo", "bar\n"},
				maxLengthBytes: 21,
				wantErr:        "component[1]: non-printable rune '\\n' at byte index 3",
			},
			{
				name:           "shortening with unicode replacement character",
				input:          []string{"foo", "bar\uFFFD"},
				maxLengthBytes: 21,
				wantErr:        "component[1]: unicode replacement character (U+FFFD) at byte index 3",
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *ftt.Test) {
				gotResult, gotResultBytes, err := shortenCaseNameComponents(tc.input, tc.maxLengthBytes)
				if tc.wantErr != "" {
					assert.Loosely(t, err, should.ErrLike(tc.wantErr))
				} else {
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, gotResult, should.Resemble(tc.wantResult))
					assert.That(t, gotResultBytes, should.Equal(tc.wantResultBytes))
				}
			})
		}
	})
}

func TestShortenStructuredID(t *testing.T) {
	t.Parallel()
	ftt.Run(`shortenStructuredID`, t, func(t *ftt.Test) {
		testCases := []struct {
			name           string
			input          *sinkpb.TestIdentifier
			maxLengthBytes int
			wantResult     *sinkpb.TestIdentifier
			wantErr        string
		}{
			{
				name: "no shortening",
				input: &sinkpb.TestIdentifier{
					CoarseName:         "foo",
					FineName:           "bar",
					CaseNameComponents: []string{strings.Repeat("a", 200)},
				},
				maxLengthBytes: 300,
				wantResult: &sinkpb.TestIdentifier{
					CoarseName:         "foo",
					FineName:           "bar",
					CaseNameComponents: []string{strings.Repeat("a", 200)},
				},
			},
			{
				name: "shorten coarse name",
				input: &sinkpb.TestIdentifier{
					CoarseName:         strings.Repeat("a", 200),
					FineName:           "bar",
					CaseNameComponents: []string{"baz"},
				},
				maxLengthBytes: 100,
				wantResult: &sinkpb.TestIdentifier{
					CoarseName:         strings.Repeat("a", 32) + "~c2a908d98f5df987",
					FineName:           "bar",
					CaseNameComponents: []string{"baz"},
				},
			},
			{
				name: "shorten fine name",
				input: &sinkpb.TestIdentifier{
					CoarseName:         "foo",
					FineName:           strings.Repeat("a", 200),
					CaseNameComponents: []string{"baz"},
				},
				maxLengthBytes: 100,
				wantResult: &sinkpb.TestIdentifier{
					CoarseName:         "foo",
					FineName:           strings.Repeat("a", 53) + "~c2a908d98f5df987",
					CaseNameComponents: []string{"baz"},
				},
			},
			{
				name: "shorten case name",
				input: &sinkpb.TestIdentifier{
					CoarseName:         "foo",
					FineName:           "bar",
					CaseNameComponents: []string{strings.Repeat("a", 200)},
				},
				maxLengthBytes: 100,
				wantResult: &sinkpb.TestIdentifier{
					CoarseName:         "foo",
					FineName:           "bar",
					CaseNameComponents: []string{strings.Repeat("a", 75) + "~c2a908d98f5df987"},
				},
			}, {
				name: "shorten all",
				input: &sinkpb.TestIdentifier{
					CoarseName:         strings.Repeat("a", 200),
					FineName:           strings.Repeat("b", 200),
					CaseNameComponents: []string{strings.Repeat("c", 200)},
				},
				maxLengthBytes: 100,
				wantResult: &sinkpb.TestIdentifier{
					CoarseName:         strings.Repeat("a", 32) + "~c2a908d98f5df987",
					FineName:           strings.Repeat("b", 7) + "~aaebc35c4c4e2cc7",
					CaseNameComponents: []string{strings.Repeat("c", 8) + "~5e39abe881f543e1"},
				},
			},
			{
				name:           "shorten with invalid UTF-8",
				input:          &sinkpb.TestIdentifier{CoarseName: "foo", FineName: "bar\x80", CaseNameComponents: []string{"baz"}},
				maxLengthBytes: 100,
				wantErr:        "fine_name: not a valid utf8 string",
			},
			{
				name:           "shorten with non-printable",
				input:          &sinkpb.TestIdentifier{CoarseName: "foo", FineName: "bar\n", CaseNameComponents: []string{"baz"}},
				maxLengthBytes: 100,
				wantErr:        "fine_name: non-printable rune '\\n' at byte index 3",
			},
			{
				name:           "shorten with unicode replacement character",
				input:          &sinkpb.TestIdentifier{CoarseName: "foo", FineName: "bar\uFFFD", CaseNameComponents: []string{"baz"}},
				maxLengthBytes: 100,
				wantErr:        "fine_name: unicode replacement character (U+FFFD) at byte index 3",
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *ftt.Test) {
				gotResult, err := shortenStructuredID(tc.input, tc.maxLengthBytes)
				if tc.wantErr != "" {
					assert.Loosely(t, err, should.ErrLike(tc.wantErr))
				} else {
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, gotResult, should.Resemble(tc.wantResult))
				}
			})
		}
	})
}

func TestShortenTestID(t *testing.T) {
	t.Parallel()
	ftt.Run(`shortenTestID`, t, func(t *ftt.Test) {
		testCases := []struct {
			name       string
			input      string
			maxBytes   int
			wantResult string
			wantErr    string
		}{
			{
				name:       "no shortening",
				input:      "fooBar",
				maxBytes:   10,
				wantResult: "fooBar",
			},
			{
				name:       "shortening",
				input:      strings.Repeat("a", 400),
				maxBytes:   100,
				wantResult: strings.Repeat("a", 83) + "~abd5e54be3f59d8e",
			},
			{
				name:       "shortening with multi-byte rune",
				input:      strings.Repeat("€", 400), // € is 0xE2 0x82 0xAC in UTF-8 (3 bytes).
				maxBytes:   100,
				wantResult: strings.Repeat("€", 27) + "~c607fb0e158d8f90",
			},
			{
				name:     "shortening with invalid UTF-8",
				input:    strings.Repeat("a", 400) + "\x80",
				maxBytes: 100,
				wantErr:  "not a valid utf8 string",
			},
			{
				name:     "shortening with non-printable",
				input:    strings.Repeat("a", 400) + "\n",
				maxBytes: 100,
				wantErr:  "non-printable rune '\\n' at byte index 400",
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *ftt.Test) {
				gotResult, err := shortenTestID(tc.input, tc.maxBytes)
				if tc.wantErr != "" {
					assert.Loosely(t, err, should.ErrLike(tc.wantErr))
				} else {
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, gotResult, should.Equal(tc.wantResult))
				}
			})
		}
	})
}
