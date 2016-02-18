// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

const (
	testDir = "testdata"
)

func TestMain(t *testing.T) {
	t.Parallel()

	Convey("svxmux", t, func() {
		c := context.Background()

		tmpDir, err := ioutil.TempDir("", "")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tmpDir)

		var g generator

		Convey("Works", func() {
			output := filepath.Join(tmpDir, "s1server_mux.go")
			err = g.run(
				c,
				"-output", output,
				"-type", "S1Server,S2Server",
				testDir,
			)
			So(err, ShouldBeNil)

			wantFile := filepath.Join(testDir, "s1server_mux.golden")
			want, err := ioutil.ReadFile(wantFile)
			So(err, ShouldBeNil)

			got, err := ioutil.ReadFile(output)
			So(err, ShouldBeNil)

			So(string(got), ShouldEqual, string(want))
		})

		Convey("Type not found", func() {
			err := g.run(c, "-type", "XServer")
			So(err, ShouldErrLike, "type XServer not found")
		})

		Convey("Embedded interface", func() {
			err := g.run(c, "-type", "CompoundServer", testDir)
			So(err, ShouldErrLike, "CompoundServer embeds S1Server")
		})
	})
}
