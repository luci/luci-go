// Copyright 2016 The LUCI Authors.
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

package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var (
	inputDir   = filepath.Join("..", "..", "internal", "svctool", "testdata")
	goldenFile = "testdata/s1server_mux.golden"
)

func TestMain(t *testing.T) {
	t.Parallel()

	Convey("svxmux", t, func() {
		tmpDir, err := ioutil.TempDir("", "")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tmpDir)

		run := func(args ...string) error {
			t := tool()
			t.ParseArgs(args)
			return t.Run(context.Background(), generate)
		}

		Convey("Works", func() {
			output := filepath.Join(tmpDir, "s1server_mux.go")
			err := run(
				"-output", output,
				"-type", "S1Server,S2Server",
				inputDir,
			)
			So(err, ShouldBeNil)

			want, err := ioutil.ReadFile(goldenFile)
			So(err, ShouldBeNil)

			got, err := ioutil.ReadFile(output)
			So(err, ShouldBeNil)

			So(string(got), ShouldEqual, string(want))
		})

		Convey("Type not found", func() {
			err := run("-type", "XServer", inputDir)
			So(err, ShouldErrLike, "type XServer not found")
		})

		Convey("Embedded interface", func() {
			err := run("-type", "CompoundServer", "testdata")
			So(err, ShouldErrLike, "CompoundServer embeds test.S1Server")
		})
	})
}
