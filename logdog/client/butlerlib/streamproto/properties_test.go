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
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

// Test the TeeType struct.
func TestProperties(t *testing.T) {
	Convey(`A testing instance`, t, func() {
		ctx, _ := testclock.UseTime(context.Background(), time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))
		Convey(`A Properties instance with no ContentType`, func() {
			p := Properties{
				LogStreamDescriptor: &logpb.LogStreamDescriptor{},
			}

			Convey(`Returns the configured ContentType if one is set.`, func() {
				p.ContentType = "foo/bar"
				p.StreamType = logpb.StreamType_TEXT
				So(p.ContentType, ShouldEqual, "foo/bar")
			})

			Convey(`Will fail to validate if a Prefix is set.`, func() {
				p.Prefix = "some/prefix"
				So(p.Validate(), ShouldNotBeNil)
			})

			Convey(`Will fail to validate if its LogStreamDescriptor is invalid.`, func() {
				So(p.Validate(), ShouldNotBeNil)
			})

			Convey(`Will validate if valid.`, func() {
				p.Name = "foo/bar"
				p.ContentType = "some/mimetype"
				p.Timestamp = google.NewTimestamp(clock.Now(ctx))
				So(p.Validate(), ShouldBeNil)
			})
		})
	})
}

func TestFlags(t *testing.T) {
	Convey(`A Flags instance`, t, func() {
		f := Flags{
			Name:        "my/stream",
			ContentType: "foo/bar",
		}

		Convey(`Converts to Properties.`, func() {
			p := f.Properties()
			So(p, ShouldResemble, &Properties{
				LogStreamDescriptor: &logpb.LogStreamDescriptor{
					Name:        "my/stream",
					ContentType: "foo/bar",
					StreamType:  logpb.StreamType_TEXT,
				},
				Tee: TeeNone,
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

			So(f.Properties(), ShouldResemble, &Properties{
				LogStreamDescriptor: &logpb.LogStreamDescriptor{
					Name:        "my/stream",
					ContentType: "foo/bar",
					StreamType:  logpb.StreamType_TEXT,
				},
				Tee: TeeNone,
			})
		})

		Convey(`Will fail to decode an invalid stream type.`, func() {
			t := `{"name": "my/stream", "type": "XXX_whatisthis?"}`
			So(json.Unmarshal([]byte(t), &f), ShouldNotBeNil)
		})

		Convey(`Will decode a BINARY stream.`, func() {
			t := `{"name": "my/stream", "type": "binary"}`
			So(json.Unmarshal([]byte(t), &f), ShouldBeNil)

			So(f.Properties(), ShouldResemble, &Properties{
				LogStreamDescriptor: &logpb.LogStreamDescriptor{
					Name:        "my/stream",
					StreamType:  logpb.StreamType_BINARY,
					ContentType: string(types.ContentTypeBinary),
				},
				Tee: TeeNone,
			})
		})

		Convey(`Will decode a DATAGRAM stream.`, func() {
			t := `{"name": "my/stream", "type": "datagram"}`
			So(json.Unmarshal([]byte(t), &f), ShouldBeNil)

			So(f.Properties(), ShouldResemble, &Properties{
				LogStreamDescriptor: &logpb.LogStreamDescriptor{
					Name:        "my/stream",
					StreamType:  logpb.StreamType_DATAGRAM,
					ContentType: string(types.ContentTypeLogdogDatagram),
				},
				Tee: TeeNone,
			})
		})
	})
}
