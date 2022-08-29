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

// Package buildtoken provide related functions for generating and parsing build
// tokens.
package buildtoken

import (
	"encoding/base64"
	"testing"

	"google.golang.org/protobuf/proto"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBuildToken(t *testing.T) {
	t.Parallel()

	Convey("build token", t, func() {
		Convey("success", func() {
			bID := int64(123)
			token, err := GenerateToken(bID, pb.TokenBody_BUILD)
			So(err, ShouldBeNil)
			tBody, err := ParseToTokenBody(token)
			So(err, ShouldBeNil)
			So(tBody.BuildId, ShouldEqual, bID)
			So(tBody.Purpose, ShouldEqual, pb.TokenBody_BUILD)
			So(len(tBody.State), ShouldNotEqual, 0)
		})

		Convey("not base64 encoded token", func() {
			_, err := ParseToTokenBody("invalid token")
			So(err, ShouldErrLike, "error decoding token")
		})

		Convey("bad base64 encoded token", func() {
			_, err := ParseToTokenBody("abckish")
			So(err, ShouldErrLike, "error unmarshalling token")
		})

		Convey("unsupported token version", func() {
			tkBody := &pb.TokenBody{
				BuildId: 1,
				State:   []byte("random"),
			}
			tkBytes, err := proto.Marshal(tkBody)
			So(err, ShouldBeNil)
			tkEnvelop := &pb.TokenEnvelope{
				Version: pb.TokenEnvelope_VERSION_UNSPECIFIED,
				Payload: tkBytes,
			}
			tkeBytes, err := proto.Marshal(tkEnvelop)
			So(err, ShouldBeNil)
			_, err = ParseToTokenBody(base64.RawURLEncoding.EncodeToString(tkeBytes))
			So(err, ShouldErrLike, "token with version 0 is not supported")
		})
	})
}
