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
	"flag"
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestOutputFlag(t *testing.T) {
	t.Parallel()

	Convey(`OutputFlag`, t, func() {
		fs := &flag.FlagSet{}
		fs.SetOutput(io.Discard)
		of := AddOutputFlagToSet(fs)

		Convey(`good`, func() {
			Convey(`empty`, func() {
				So(fs.Parse(nil), ShouldBeNil)
				So(of.Path, ShouldBeEmpty)
				So(of.Codec, ShouldResemble, buildFileCodecNoop{})
			})

			Convey(`missing`, func() {
				So(fs.Parse([]string{"other", "stuff"}), ShouldBeNil)
				So(of.Path, ShouldBeEmpty)
				So(of.Codec, ShouldResemble, buildFileCodecNoop{})
			})

			Convey(`equal`, func() {
				So(fs.Parse([]string{OutputCLIArg + "=out.textpb", "a", "b"}), ShouldBeNil)
				So(of.Path, ShouldResemble, "out.textpb")
				So(of.Codec, ShouldResemble, buildFileCodecText{})
				So(fs.Args(), ShouldResemble, []string{"a", "b"})
			})

			Convey(`two val`, func() {
				So(fs.Parse([]string{OutputCLIArg, "out.json", "a", "b"}), ShouldBeNil)
				So(of.Path, ShouldResemble, "out.json")
				So(of.Codec, ShouldResemble, buildFileCodecJSON{})
				So(fs.Args(), ShouldResemble, []string{"a", "b"})
			})

			Convey(`explicit noop`, func() {
				So(fs.Parse([]string{OutputCLIArg + "="}), ShouldBeNil)
				So(of.Path, ShouldBeEmpty)
				So(of.Codec, ShouldResemble, buildFileCodecNoop{})
			})

		})

		Convey(`bad`, func() {
			Convey(`incomplete`, func() {
				So(fs.Parse([]string{OutputCLIArg}), ShouldErrLike, "flag needs an argument")
				So(of.Path, ShouldBeEmpty)
				So(of.Codec, ShouldResemble, buildFileCodecNoop{})
			})

			Convey(`bad extension`, func() {
				So(fs.Parse([]string{OutputCLIArg, "nope", "arg"}), ShouldErrLike, "bad extension")
				So(of.Path, ShouldBeEmpty)
				So(of.Codec, ShouldResemble, buildFileCodecNoop{})
			})
		})
	})
}
