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

package exe

import (
	"bytes"
	"compress/zlib"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/luciexe"
	"go.chromium.org/luci/luciexe/exe/exeutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestExe(t *testing.T) {
	t.Parallel()

	Convey(`test exe`, t, func() {
		client := streamclient.NewFake("test_namespace")

		getBuilds := func(decompress bool) []*bbpb.Build {
			fakeData := client.GetFakeData()["test_namespace/build.proto"]
			if decompress {
				So(fakeData.GetFlags().ContentType, ShouldEqual, luciexe.BuildProtoZlibContentType)
			} else {
				So(fakeData.GetFlags().ContentType, ShouldEqual, luciexe.BuildProtoContentType)
			}

			dgs := fakeData.GetDatagrams()
			So(len(dgs), ShouldBeGreaterThanOrEqualTo, 1)

			ret := make([]*bbpb.Build, len(dgs))
			for i, dg := range dgs {
				ret[i] = &bbpb.Build{}

				var data []byte

				if decompress {
					r, err := zlib.NewReader(bytes.NewBufferString(dg))
					So(err, ShouldBeNil)
					data, err = ioutil.ReadAll(r)
					So(err, ShouldBeNil)
				} else {
					data = []byte(dg)
				}

				So(proto.Unmarshal(data, ret[i]), ShouldBeNil)
			}
			return ret
		}
		lastBuild := func() *bbpb.Build {
			builds := getBuilds(false)
			return builds[len(builds)-1]
		}

		args := []string{"fake_test_executable"}

		opts := []RunOption{OptLogdogClient(client.Client)}

		Convey(`basic`, func() {
			Convey(`success`, func() {
				exitCode := runImpl(args, opts, func(ctx context.Context, build *Build, userArgs []string) error {
					return nil
				})
				So(exitCode, ShouldEqual, 0)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status: bbpb.Status_SUCCESS,
					Output: &bbpb.Build_Output{
						Properties: &structpb.Struct{},
					},
				})
			})

			Convey(`failure`, func() {
				exitCode := runImpl(args, opts, func(ctx context.Context, build *Build, userArgs []string) error {
					return errors.New("bad stuff")
				})
				So(exitCode, ShouldEqual, 1)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status: bbpb.Status_FAILURE,
					Output: &bbpb.Build_Output{
						Properties: &structpb.Struct{},
					},
				})
			})

			Convey(`infra failure`, func() {
				exitCode := runImpl(args, opts, func(ctx context.Context, build *Build, userArgs []string) error {
					return errors.New("bad stuff", StatusInfraFailure)
				})
				So(exitCode, ShouldEqual, 1)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status: bbpb.Status_INFRA_FAILURE,
					Output: &bbpb.Build_Output{
						Properties: &structpb.Struct{},
					},
				})
			})

			Convey(`resource exhuastion/timeout failure`, func() {
				exitCode := runImpl(args, opts, func(ctx context.Context, build *Build, userArgs []string) error {
					return errors.New("bad stuff", StatusDetailTimeout, StatusDetailResourceExhaustion)
				})
				So(exitCode, ShouldEqual, 1)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status: bbpb.Status_FAILURE,
					StatusDetails: &bbpb.StatusDetails{
						Timeout:            &bbpb.StatusDetails_Timeout{},
						ResourceExhaustion: &bbpb.StatusDetails_ResourceExhaustion{},
					},
					Output: &bbpb.Build_Output{
						Properties: &structpb.Struct{},
					},
				})
			})

			Convey(`panic`, func() {
				exitCode := runImpl(args, opts, func(ctx context.Context, build *Build, userArgs []string) error {
					panic(errors.New("bad stuff"))
				})
				So(exitCode, ShouldEqual, 2)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status: bbpb.Status_INFRA_FAILURE,
					Output: &bbpb.Build_Output{
						Properties: &structpb.Struct{},
					},
				})
			})
		})

		Convey(`send`, func() {
			exitCode := runImpl(args, opts, func(ctx context.Context, build *Build, userArgs []string) error {
				return build.Modify(func(bv *BuildView) error {
					bv.SummaryMarkdown = "Hi. I did stuff."
					return errors.New("oh no i failed")
				})
			})
			So(exitCode, ShouldEqual, 1)
			builds := getBuilds(false)
			So(builds[len(builds)-1], ShouldResembleProto, &bbpb.Build{
				Status:          bbpb.Status_FAILURE,
				SummaryMarkdown: "Hi. I did stuff.",
				Output: &bbpb.Build_Output{
					Properties: &structpb.Struct{},
				},
			})
		})

		Convey(`send (zlib)`, func() {
			opts = append(opts, OptZlibCompression(5))
			exitCode := runImpl(args, opts, func(ctx context.Context, build *Build, userArgs []string) error {
				return build.Modify(func(bv *BuildView) error {
					bv.SummaryMarkdown = "Hi. I did stuff."
					return errors.New("oh no i failed")
				})
			})
			So(exitCode, ShouldEqual, 1)
			builds := getBuilds(true)
			So(builds[len(builds)-1], ShouldResembleProto, &bbpb.Build{
				Status:          bbpb.Status_FAILURE,
				SummaryMarkdown: "Hi. I did stuff.",
				Output: &bbpb.Build_Output{
					Properties: &structpb.Struct{},
				},
			})
		})

		Convey(`output`, func() {
			tdir, err := ioutil.TempDir("", "luciexe-exe-test")
			So(err, ShouldBeNil)
			defer os.RemoveAll(tdir)

			Convey(`binary`, func() {
				outFile := filepath.Join(tdir, "out.pb")
				args = append(args, luciexe.OutputCLIArg, outFile)
				exitCode := runImpl(args, opts, func(ctx context.Context, build *Build, userArgs []string) error {
					build.Modify(func(bv *BuildView) error {
						bv.SummaryMarkdown = "Hi."
						return nil
					})
					return ModifyProperties(ctx, func(props *structpb.Struct) error {
						return exeutil.WriteProperties(props, map[string]interface{}{
							"some": "thing",
						})
					})
				})
				So(exitCode, ShouldEqual, 0)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status:          bbpb.Status_SUCCESS,
					SummaryMarkdown: "Hi.",
					Output: &bbpb.Build_Output{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"some": {Kind: &structpb.Value_StringValue{
									StringValue: "thing",
								}},
							},
						},
					},
				})
				data, err := ioutil.ReadFile(outFile)
				So(err, ShouldBeNil)
				So(string(data), ShouldResemble,
					"`\f\x82\x01\x13\n\x11\n\x0f\n\x04some\x12\a\x1a\x05thing\xa2\x01\x03Hi.")
			})

			Convey(`textpb`, func() {
				outFile := filepath.Join(tdir, "out.textpb")
				args = append(args, luciexe.OutputCLIArg, outFile)
				exitCode := runImpl(args, opts, func(ctx context.Context, build *Build, userArgs []string) error {
					return build.Modify(func(bv *BuildView) error {
						bv.SummaryMarkdown = "Hi."
						return nil
					})
				})
				So(exitCode, ShouldEqual, 0)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status:          bbpb.Status_SUCCESS,
					SummaryMarkdown: "Hi.",
					Output: &bbpb.Build_Output{
						Properties: &structpb.Struct{},
					},
				})
				data, err := ioutil.ReadFile(outFile)
				So(err, ShouldBeNil)
				So(string(data), ShouldResemble,
					"status: SUCCESS\nsummary_markdown: \"Hi.\"\noutput: <\n  properties: <\n  >\n>\n")
			})

			Convey(`jsonpb`, func() {
				outFile := filepath.Join(tdir, "out.json")
				args = append(args, luciexe.OutputCLIArg, outFile)
				exitCode := runImpl(args, opts, func(ctx context.Context, build *Build, userArgs []string) error {
					return build.Modify(func(bv *BuildView) error {
						bv.SummaryMarkdown = "Hi."
						return nil
					})
				})
				So(exitCode, ShouldEqual, 0)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status:          bbpb.Status_SUCCESS,
					SummaryMarkdown: "Hi.",
					Output: &bbpb.Build_Output{
						Properties: &structpb.Struct{},
					},
				})
				data, err := ioutil.ReadFile(outFile)
				So(err, ShouldBeNil)
				So(string(data), ShouldResemble,
					"{\n  \"status\": \"SUCCESS\",\n  \"summary_markdown\": \"Hi.\",\n  \"output\": {\n    \"properties\": {\n      }\n  }\n}")
			})

			Convey(`pass through user args`, func() {
				// Delimiter inside user args should also be passed through
				expectedUserArgs := []string{"foo", "bar", ArgsDelim, "baz"}
				Convey(`when output is not specified`, func() {
					args = append(args, ArgsDelim)
					args = append(args, expectedUserArgs...)
					exitcode := runImpl(args, opts, func(ctx context.Context, build *Build, userArgs []string) error {
						So(userArgs, ShouldResemble, expectedUserArgs)
						return nil
					})
					So(exitcode, ShouldEqual, 0)
				})
				Convey(`when output is specified`, func() {
					tdir, err := ioutil.TempDir("", "luciexe-exe-test")
					So(err, ShouldBeNil)
					defer os.RemoveAll(tdir)
					args = append(args, luciexe.OutputCLIArg, filepath.Join(tdir, "out.pb"), ArgsDelim)
					args = append(args, expectedUserArgs...)
					exitcode := runImpl(args, opts, func(ctx context.Context, build *Build, userArgs []string) error {
						So(userArgs, ShouldResemble, expectedUserArgs)
						return nil
					})
					So(exitcode, ShouldEqual, 0)
				})
			})
		})
	})
}
