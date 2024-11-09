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

package cipd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/common"
)

func TestUnixTime(t *testing.T) {
	t.Parallel()

	ftt.Run("Zero works", t, func(t *ftt.Test) {
		z := UnixTime{}

		assert.Loosely(t, z.IsZero(), should.BeTrue)
		assert.Loosely(t, z.String(), should.Equal("0001-01-01 00:00:00 +0000 UTC"))

		buf, err := json.Marshal(map[string]any{
			"key": z,
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(buf), should.Equal(`{"key":0}`))
	})

	ftt.Run("Non-zero works", t, func(t *ftt.Test) {
		t1 := UnixTime(time.Unix(1530934723, 0).UTC())

		assert.Loosely(t, t1.IsZero(), should.BeFalse)
		assert.Loosely(t, t1.String(), should.Equal("2018-07-07 03:38:43 +0000 UTC"))

		buf, err := json.Marshal(map[string]any{
			"key": t1,
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(buf), should.Equal(`{"key":1530934723}`))

		assert.Loosely(t, t1.Before(UnixTime{}), should.BeFalse)
	})
}

func TestJSONError(t *testing.T) {
	t.Parallel()

	ftt.Run("Nil works", t, func(t *ftt.Test) {
		buf, err := json.Marshal(map[string]any{
			"key": JSONError{},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(buf), should.Equal(`{"key":null}`))
	})

	ftt.Run("Non-nil works", t, func(t *ftt.Test) {
		buf, err := json.Marshal(map[string]any{
			"key": JSONError{fmt.Errorf("some error message")},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(buf), should.Equal(`{"key":"some error message"}`))
	})
}

func TestApiDescToInfo(t *testing.T) {
	t.Parallel()

	ts := time.Unix(1530934723, 0).UTC()

	apiInst := &api.Instance{
		Package: "a",
		Instance: &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: strings.Repeat("0", 64),
		},
		RegisteredBy: "user:a@example.com",
		RegisteredTs: timestamppb.New(ts),
	}

	instInfo := InstanceInfo{
		Pin: common.Pin{
			PackageName: "a",
			InstanceID:  common.ObjectRefToInstanceID(apiInst.Instance),
		},
		RegisteredBy: "user:a@example.com",
		RegisteredTs: UnixTime(ts),
	}

	ftt.Run("Only instance info", t, func(t *ftt.Test) {
		assert.Loosely(t,
			apiDescToInfo(&api.DescribeInstanceResponse{Instance: apiInst}),
			should.Resemble(
				&InstanceDescription{InstanceInfo: instInfo},
			))
	})

	ftt.Run("With tags and refs", t, func(t *ftt.Test) {
		in := &api.DescribeInstanceResponse{
			Instance: apiInst,
			Tags: []*api.Tag{
				{
					Key:        "k",
					Value:      "v",
					AttachedBy: "user:t@example.com",
					AttachedTs: timestamppb.New(ts.Add(time.Second)),
				},
			},
			Refs: []*api.Ref{
				{
					Name:       "ref",
					Instance:   apiInst.Instance,
					ModifiedBy: "user:r@example.com",
					ModifiedTs: timestamppb.New(ts.Add(2 * time.Second)),
				},
			},
		}
		assert.Loosely(t, apiDescToInfo(in), should.Resemble(&InstanceDescription{
			InstanceInfo: instInfo,
			Tags: []TagInfo{
				{
					Tag:          "k:v",
					RegisteredBy: "user:t@example.com",
					RegisteredTs: UnixTime(ts.Add(time.Second)),
				},
			},
			Refs: []RefInfo{
				{
					Ref:        "ref",
					InstanceID: instInfo.Pin.InstanceID,
					ModifiedBy: "user:r@example.com",
					ModifiedTs: UnixTime(ts.Add(2 * time.Second)),
				},
			},
		}))
	})
}
