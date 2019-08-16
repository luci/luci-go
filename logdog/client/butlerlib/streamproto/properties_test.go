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

package streamproto

import (
	"encoding/json"
	"testing"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFlags(t *testing.T) {
	Convey(`A Flags instance`, t, func() {
		f := Flags{
			Name:        "my/stream",
			ContentType: "foo/bar",
		}

		Convey(`Converts to LogStreamDescriptor.`, func() {
			So(f.Descriptor(), ShouldResemble, &logpb.LogStreamDescriptor{
				Name:        "my/stream",
				ContentType: "foo/bar",
				StreamType:  logpb.StreamType_TEXT,
			})
		})
	})
}

func TestFlagsJSON(t *testing.T) {
	Convey(`A Flags instance`, t, func() {
		f := Flags{}
		Convey(`Will decode a TEXT stream.`, func() {
			t := `{"name": "my/stream", "contentType": "foo/bar", "type": "text"}`
			So(json.Unmarshal([]byte(t), &f), ShouldBeNil)

			So(f.Descriptor(), ShouldResemble, &logpb.LogStreamDescriptor{
				Name:        "my/stream",
				ContentType: "foo/bar",
				StreamType:  logpb.StreamType_TEXT,
			})
		})

		Convey(`Will fail to decode an invalid stream type.`, func() {
			t := `{"name": "my/stream", "type": "XXX_whatisthis?"}`
			So(json.Unmarshal([]byte(t), &f), ShouldNotBeNil)
		})

		Convey(`Will decode a BINARY stream.`, func() {
			t := `{"name": "my/stream", "type": "binary"}`
			So(json.Unmarshal([]byte(t), &f), ShouldBeNil)

			So(f.Descriptor(), ShouldResemble, &logpb.LogStreamDescriptor{
				Name:        "my/stream",
				StreamType:  logpb.StreamType_BINARY,
				ContentType: string(types.ContentTypeBinary),
			})
		})

		Convey(`Will decode a DATAGRAM stream.`, func() {
			t := `{"name": "my/stream", "type": "datagram"}`
			So(json.Unmarshal([]byte(t), &f), ShouldBeNil)

			So(f.Descriptor(), ShouldResemble, &logpb.LogStreamDescriptor{
				Name:        "my/stream",
				StreamType:  logpb.StreamType_DATAGRAM,
				ContentType: string(types.ContentTypeLogdogDatagram),
			})
		})
	})
}
