// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/proto/google/descriptor"
	"golang.org/x/net/context"

	"runtime"

	. "github.com/smartystreets/goconvey/convey"
)

const (
	testDataDir = "testdata"
)

func TestMain(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" || runtime.GOARCH == "386" {
		t.Skipf("we don't have protoc these build machines.")
	}

	Convey("Main", t, func() {
		testDataDirF, err := os.Open(testDataDir)
		So(err, ShouldBeNil)
		defer testDataDirF.Close()

		testCaseDirs, err := testDataDirF.Readdirnames(0)
		So(err, ShouldBeNil)

		runOn := func(sourceDir string, test func(tmpDir string)) {
			tmpDir, err := ioutil.TempDir("", "prpcgen-test")
			So(err, ShouldBeNil)
			defer os.RemoveAll(tmpDir)

			names, err := findProtoFiles(sourceDir)
			So(err, ShouldBeNil)
			So(names, ShouldNotBeEmpty)
			for _, name := range names {
				name = filepath.Base(name)
				err = copyFile(
					filepath.Join(sourceDir, name),
					filepath.Join(tmpDir, name))
				So(err, ShouldBeNil)
			}

			// Run prpcgen.
			err = run(context.Background(), tmpDir)
			So(err, ShouldBeNil)

			test(tmpDir)
		}

		for _, testDir := range testCaseDirs {
			Convey("Check generated .go files for "+testDir, func() {
				testDir := filepath.Join(testDataDir, testDir)
				runOn(testDir, func(tmpDir string) {
					goldenFiles, err := filepath.Glob(filepath.Join(testDir, "*.golden"))
					So(err, ShouldBeNil)
					So(goldenFiles, ShouldNotBeEmpty)
					for _, golden := range goldenFiles {
						want, err := ioutil.ReadFile(golden)
						So(err, ShouldBeNil)

						gotFile := strings.TrimSuffix(filepath.Base(golden), ".golden") + ".go"
						gotFile = filepath.Join(tmpDir, gotFile)
						got, err := ioutil.ReadFile(gotFile)

						So(err, ShouldBeNil)
						So(string(got), ShouldEqual, string(want))
					}
				})
			})
		}

		Convey("Check generated package.desc file.", func() {
			runOn(filepath.Join(testDataDir, "helloworld"), func(tmpDir string) {
				descBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "package.desc"))
				So(err, ShouldBeNil)
				var descs descriptor.FileDescriptorSet
				err = proto.Unmarshal(descBytes, &descs)
				So(err, ShouldBeNil)

				file := descs.FindFile("test.proto")
				So(file, ShouldNotBeNil)

				greeter := file.FindService("Greeter")
				So(greeter, ShouldNotBeNil)

				sayHello := greeter.FindMethod("SayHello")
				So(sayHello, ShouldNotBeNil)

				So(sayHello.GetInputType(), ShouldEqual, ".test.HelloRequest")
				So(sayHello.GetOutputType(), ShouldEqual, ".test.HelloReply")

				helloRequest := file.FindMessage("HelloRequest")
				So(helloRequest, ShouldNotBeNil)
				name := helloRequest.FindField("name")
				So(name, ShouldNotBeNil)

				helloReply := file.FindMessage("HelloReply")
				So(helloReply, ShouldNotBeNil)
				message := helloReply.FindField("message")
				So(message, ShouldNotBeNil)
			})
		})
	})
}

func copyFile(src, dest string) error {
	data, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(dest, data, 0666)
}
