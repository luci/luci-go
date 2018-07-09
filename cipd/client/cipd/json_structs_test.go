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

	"go.chromium.org/luci/common/proto/google"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/common"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUnixTime(t *testing.T) {
	t.Parallel()

	Convey("Zero works", t, func() {
		z := UnixTime{}

		So(z.IsZero(), ShouldBeTrue)
		So(z.String(), ShouldEqual, "0001-01-01 00:00:00 +0000 UTC")

		buf, err := json.Marshal(map[string]interface{}{
			"key": z,
		})
		So(err, ShouldBeNil)
		So(string(buf), ShouldEqual, `{"key":0}`)
	})

	Convey("Non-zero works", t, func() {
		t1 := UnixTime(time.Unix(1530934723, 0).UTC())

		So(t1.IsZero(), ShouldBeFalse)
		So(t1.String(), ShouldEqual, "2018-07-07 03:38:43 +0000 UTC")

		buf, err := json.Marshal(map[string]interface{}{
			"key": t1,
		})
		So(err, ShouldBeNil)
		So(string(buf), ShouldEqual, `{"key":1530934723}`)

		So(t1.Before(UnixTime{}), ShouldBeFalse)
	})
}

func TestJSONError(t *testing.T) {
	t.Parallel()

	Convey("Nil works", t, func() {
		buf, err := json.Marshal(map[string]interface{}{
			"key": JSONError{},
		})
		So(err, ShouldBeNil)
		So(string(buf), ShouldEqual, `{"key":null}`)
	})

	Convey("Non-nil works", t, func() {
		buf, err := json.Marshal(map[string]interface{}{
			"key": JSONError{fmt.Errorf("some error message")},
		})
		So(err, ShouldBeNil)
		So(string(buf), ShouldEqual, `{"key":"some error message"}`)
	})
}

func TestApiDescToInfo(t *testing.T) {
	t.Parallel()

	ts := time.Unix(1530934723, 0).UTC()

	apiInst := &api.Instance{
		Package: "a",
		Instance: &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: strings.Repeat("0", 40),
		},
		RegisteredBy: "user:a@example.com",
		RegisteredTs: google.NewTimestamp(ts),
	}

	instInfo := InstanceInfo{
		Pin: common.Pin{
			PackageName: "a",
			InstanceID:  strings.Repeat("0", 40),
		},
		RegisteredBy: "user:a@example.com",
		RegisteredTs: UnixTime(ts),
	}

	Convey("Only instance info", t, func() {
		So(
			apiDescToInfo(&api.DescribeInstanceResponse{Instance: apiInst}),
			ShouldResemble,
			&InstanceDescription{InstanceInfo: instInfo},
		)
	})

	Convey("With tags and refs", t, func() {
		in := &api.DescribeInstanceResponse{
			Instance: apiInst,
			Tags: []*api.Tag{
				{
					Key:        "k",
					Value:      "v",
					AttachedBy: "user:t@example.com",
					AttachedTs: google.NewTimestamp(ts.Add(time.Second)),
				},
			},
			Refs: []*api.Ref{
				{
					Name:       "ref",
					Instance:   apiInst.Instance,
					ModifiedBy: "user:r@example.com",
					ModifiedTs: google.NewTimestamp(ts.Add(2 * time.Second)),
				},
			},
		}
		So(apiDescToInfo(in), ShouldResemble, &InstanceDescription{
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
		})
	})
}
