// Copyright 2015 The LUCI Authors.
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

package internal

import (
	"testing"

	"go.chromium.org/luci/cipd/client/cipd/internal/messages"

	"github.com/golang/protobuf/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestChecksumCheckingWorks(t *testing.T) {
	msg := messages.TagCache{
		Entries: []*messages.TagCache_Entry{
			{
				Package:    "package",
				Tag:        "tag",
				InstanceId: "instance_id",
			},
		},
	}

	Convey("Works", t, func(c C) {
		buf, err := MarshalWithSHA256(&msg)
		So(err, ShouldBeNil)
		out := messages.TagCache{}
		So(UnmarshalWithSHA256(buf, &out), ShouldBeNil)
		So(&out, ShouldResembleProto, &msg)
	})

	Convey("Rejects bad msg", t, func(c C) {
		buf, err := MarshalWithSHA256(&msg)
		So(err, ShouldBeNil)
		buf[10] = 0
		out := messages.TagCache{}
		So(UnmarshalWithSHA256(buf, &out), ShouldNotBeNil)
	})

	Convey("Skips empty sha256", t, func(c C) {
		buf, err := proto.Marshal(&messages.BlobWithSHA256{Blob: []byte{1, 2, 3}})
		So(err, ShouldBeNil)
		out := messages.TagCache{}
		So(UnmarshalWithSHA256(buf, &out), ShouldEqual, ErrUnknownSHA256)
	})
}
