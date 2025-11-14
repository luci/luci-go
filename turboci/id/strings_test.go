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

package id

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
)

func shouldWrap[I Identifier](ident I, err error) (*idspb.Identifier, error) {
	if err != nil {
		return nil, err
	}
	return Wrap(ident), nil
}

func TestToFromString(t *testing.T) {
	t.Parallel()

	type testCase struct {
		expect  string
		mkIdent func() (*idspb.Identifier, error)
	}

	tcs := []testCase{
		{":Cmeep", func() (*idspb.Identifier, error) {
			return shouldWrap(CheckErr("meep"))
		}},
		{":Cmeep:O2", func() (*idspb.Identifier, error) {
			return shouldWrap(CheckOptionErr("meep", 2))
		}},
		{":Cmeep:R2", func() (*idspb.Identifier, error) {
			return shouldWrap(CheckResultErr("meep", 2))
		}},
		{":Cmeep:R2:D3", func() (*idspb.Identifier, error) {
			return shouldWrap(CheckResultDatumErr("meep", 2, 3))
		}},
		{":Cmeep:V12345/6789", func() (*idspb.Identifier, error) {
			ts := time.Unix(12345, 6789)
			return shouldWrap(CheckEditErr("meep", ts))
		}},
		{":Cmeep:V12345/6789:O10", func() (*idspb.Identifier, error) {
			ts := time.Unix(12345, 6789)
			return shouldWrap(CheckEditOptionErr("meep", ts, 10))
		}},
		{":Smeep", func() (*idspb.Identifier, error) {
			return shouldWrap(StageErr(StageNotWorknode, "meep"))
		}},
		{":?meep", func() (*idspb.Identifier, error) {
			return shouldWrap(StageErr(StageIsUnknown, "meep"))
		}},
		{":Nmeep", func() (*idspb.Identifier, error) {
			return shouldWrap(StageErr(StageIsWorknode, "meep"))
		}},
		{":Smeep:A2", func() (*idspb.Identifier, error) {
			return shouldWrap(StageAttemptErr(StageNotWorknode, "meep", 2))
		}},
		{":Nmeep:A2", func() (*idspb.Identifier, error) {
			return shouldWrap(StageAttemptErr(StageIsWorknode, "meep", 2))
		}},
		{":Smeep:V12345/6789", func() (*idspb.Identifier, error) {
			ts := time.Unix(12345, 6789)
			return shouldWrap(StageEditErr(StageNotWorknode, "meep", ts))
		}},
		{":Nmeep:V12345/6789", func() (*idspb.Identifier, error) {
			ts := time.Unix(12345, 6789)
			return shouldWrap(StageEditErr(StageIsWorknode, "meep", ts))
		}},
	}

	for _, tc := range tcs {
		t.Run(tc.expect, func(t *testing.T) {
			t.Parallel()
			id, err := tc.mkIdent()
			assert.NoErr(t, err)

			assert.That(t, ToString(id), should.Equal(tc.expect))

			ident, err := FromString(tc.expect)
			assert.That(t, ident, should.Match(id))
		})

		const wp = "00012345"
		wpExpect := "L" + wp + tc.expect
		t.Run(wpExpect, func(t *testing.T) {
			t.Parallel()
			id, err := tc.mkIdent()
			assert.NoErr(t, err)
			_, err = SetWorkplanErr(id, wp)
			assert.NoErr(t, err)

			assert.That(t, ToString(id), should.Equal(wpExpect))

			ident, err := FromString(wpExpect)
			assert.That(t, ident, should.Match(id))
		})
	}
}
