// Copyright 2019 The LUCI Authors.
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

package luciexe

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBuildCodec(t *testing.T) {
	t.Parallel()

	Convey(`TestBuildCodec`, t, func() {
		b := &bbpb.Build{
			Id:              100,
			SummaryMarkdown: "stuff",
		}

		Convey(`bad ext`, func() {
			codec, err := BuildFileCodecForPath("blah.bad")
			So(err, ShouldErrLike, "bad extension")
			So(codec, ShouldBeNil)
		})

		Convey(`noop`, func() {
			codec, err := BuildFileCodecForPath("")
			So(err, ShouldBeNil)
			So(codec, ShouldResemble, buildFileCodecNoop{})
			So(codec.IsNoop(), ShouldBeTrue)
			So(codec.FileExtension(), ShouldResemble, "")
			So(codec.Enc(nil, nil), ShouldBeNil)
			So(codec.Dec(nil, nil), ShouldBeNil)
		})

		Convey(`json`, func() {
			codec, err := BuildFileCodecForPath("blah.json")
			So(err, ShouldBeNil)
			So(codec, ShouldResemble, buildFileCodecJSON{})
			So(codec.IsNoop(), ShouldBeFalse)
			So(codec.FileExtension(), ShouldResemble, ".json")

			buf := &bytes.Buffer{}
			So(codec.Enc(b, buf), ShouldBeNil)
			So(buf.String(), ShouldResemble,
				"{\n  \"id\": \"100\",\n  \"summary_markdown\": \"stuff\"\n}")

			outBuild := &bbpb.Build{}
			So(codec.Dec(outBuild, buf), ShouldBeNil)
			So(outBuild, ShouldResembleProto, b)
		})

		Convey(`textpb`, func() {
			codec, err := BuildFileCodecForPath("blah.textpb")
			So(err, ShouldBeNil)
			So(codec, ShouldResemble, buildFileCodecText{})
			So(codec.IsNoop(), ShouldBeFalse)
			So(codec.FileExtension(), ShouldResemble, ".textpb")

			buf := &bytes.Buffer{}
			So(codec.Enc(b, buf), ShouldBeNil)
			So(buf.String(), ShouldResemble,
				"id: 100\nsummary_markdown: \"stuff\"\n")

			outBuild := &bbpb.Build{}
			So(codec.Dec(outBuild, buf), ShouldBeNil)
			So(outBuild, ShouldResembleProto, b)
		})

		Convey(`pb`, func() {
			codec, err := BuildFileCodecForPath("blah.pb")
			So(err, ShouldBeNil)
			So(codec, ShouldResemble, buildFileCodecBinary{})
			So(codec.IsNoop(), ShouldBeFalse)
			So(codec.FileExtension(), ShouldResemble, ".pb")

			buf := &bytes.Buffer{}
			So(codec.Enc(b, buf), ShouldBeNil)
			So(buf.Bytes(), ShouldResemble,
				[]byte{8, 100, 162, 1, 5, 115, 116, 117, 102, 102})

			outBuild := &bbpb.Build{}
			So(codec.Dec(outBuild, buf), ShouldBeNil)
			So(outBuild, ShouldResembleProto, b)
		})
	})
}
