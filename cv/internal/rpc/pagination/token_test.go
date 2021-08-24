// Copyright 2021 The LUCI Authors.
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

package pagination

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPageTokens(t *testing.T) {
	t.Parallel()

	Convey("Page token round trip", t, func() {
		Convey("not empty", func() {
			// Any proto type should work.
			in := timestamppb.New(testclock.TestRecentTimeUTC)
			token, err := TokenString(in)
			So(err, ShouldBeNil)
			out := &timestamppb.Timestamp{}
			So(parse(token, out), ShouldBeNil)
			So(out, ShouldResembleProto, in)
		})
		Convey("empty page token", func() {
			token, err := TokenString(nil)
			So(err, ShouldBeNil)
			So(token, ShouldResemble, "")
			out := &timestamppb.Timestamp{}
			So(parse(token, out), ShouldBeNil)
			So(out, ShouldResembleProto, &timestamppb.Timestamp{})
		})
		Convey("empty page token with typed nil", func() {
			var typedNil *timestamppb.Timestamp
			token, err := TokenString(typedNil)
			So(err, ShouldBeNil)
			So(token, ShouldResemble, "")
		})
	})

	Convey("Page token parsing errors", t, func() {
		dst := &timestamppb.Timestamp{Seconds: 1, Nanos: 2}
		goodToken, err := TokenString(timestamppb.New(testclock.TestRecentTimeUTC))
		So(err, ShouldBeNil)

		Convey("bad base64", func() {
			tokenBytes := []byte(goodToken)
			tokenBytes[10] = '\\'
			err = parse(string(tokenBytes), dst)
			So(err, ShouldErrLike, "illegal base64")
			So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page token")
		})

		Convey("bad proto", func() {
			tokenBytes := []byte(goodToken)
			tokenBytes[4] = 'a'
			err = parse(string(tokenBytes), dst)
			So(err, ShouldErrLike, "invalid wire-format")
			So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page token")
		})

		Convey("bad version", func() {
			err := parse("blah", dst)
			So(err, ShouldErrLike, "unknown version")
			So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page token")
		})
	})
}
