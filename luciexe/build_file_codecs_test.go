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

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestBuildCodec(t *testing.T) {
	t.Parallel()

	ftt.Run(`TestBuildCodec`, t, func(t *ftt.Test) {
		b := &bbpb.Build{
			Id:              100,
			SummaryMarkdown: "stuff",
		}

		t.Run(`bad ext`, func(t *ftt.Test) {
			codec, err := BuildFileCodecForPath("blah.bad")
			assert.Loosely(t, err, should.ErrLike("bad extension"))
			assert.Loosely(t, codec, should.BeNil)
		})

		t.Run(`noop`, func(t *ftt.Test) {
			codec, err := BuildFileCodecForPath("")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, codec, should.Match(buildFileCodecNoop{}))
			assert.Loosely(t, codec.IsNoop(), should.BeTrue)
			assert.Loosely(t, codec.FileExtension(), should.BeBlank)
			assert.Loosely(t, codec.Enc(nil, nil), should.BeNil)
			assert.Loosely(t, codec.Dec(nil, nil), should.BeNil)
		})

		t.Run(`json`, func(t *ftt.Test) {
			codec, err := BuildFileCodecForPath("blah.json")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, codec, should.Match(buildFileCodecJSON{}))
			assert.Loosely(t, codec.IsNoop(), should.BeFalse)
			assert.Loosely(t, codec.FileExtension(), should.Match(".json"))

			buf := &bytes.Buffer{}
			assert.Loosely(t, codec.Enc(b, buf), should.BeNil)
			assert.Loosely(t, buf.String(), should.Match(
				"{\n  \"id\": \"100\",\n  \"summary_markdown\": \"stuff\"\n}"))

			outBuild := &bbpb.Build{}
			assert.Loosely(t, codec.Dec(outBuild, buf), should.BeNil)
			assert.Loosely(t, outBuild, should.Match(b))
		})

		t.Run(`textpb`, func(t *ftt.Test) {
			codec, err := BuildFileCodecForPath("blah.textpb")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, codec, should.Match(buildFileCodecText{}))
			assert.Loosely(t, codec.IsNoop(), should.BeFalse)
			assert.Loosely(t, codec.FileExtension(), should.Match(".textpb"))

			buf := &bytes.Buffer{}
			assert.Loosely(t, codec.Enc(b, buf), should.BeNil)
			assert.Loosely(t, buf.String(), should.Match(
				"id: 100\nsummary_markdown: \"stuff\"\n"))

			outBuild := &bbpb.Build{}
			assert.Loosely(t, codec.Dec(outBuild, buf), should.BeNil)
			assert.Loosely(t, outBuild, should.Match(b))
		})

		t.Run(`pb`, func(t *ftt.Test) {
			codec, err := BuildFileCodecForPath("blah.pb")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, codec, should.Match(buildFileCodecBinary{}))
			assert.Loosely(t, codec.IsNoop(), should.BeFalse)
			assert.Loosely(t, codec.FileExtension(), should.Match(".pb"))

			buf := &bytes.Buffer{}
			assert.Loosely(t, codec.Enc(b, buf), should.BeNil)
			assert.Loosely(t, buf.Bytes(), should.Match(
				[]byte{8, 100, 162, 1, 5, 115, 116, 117, 102, 102}))

			outBuild := &bbpb.Build{}
			assert.Loosely(t, codec.Dec(outBuild, buf), should.BeNil)
			assert.Loosely(t, outBuild, should.Match(b))
		})
	})
}
