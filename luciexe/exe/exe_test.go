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
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/luciexe"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestExe(t *testing.T) {
	t.Parallel()

	Convey(`test exe`, t, func() {
		scFake := streamclient.NewFake()
		defer scFake.Unregister()

		env := environ.New(nil)
		env.Set(bootstrap.EnvCoordinatorHost, "test.example.com")
		env.Set(bootstrap.EnvStreamProject, "test_project")
		env.Set(bootstrap.EnvStreamPrefix, "test_prefix")
		env.Set(bootstrap.EnvNamespace, "test_namespace")
		env.Set(bootstrap.EnvStreamServerPath, scFake.StreamServerPath())
		ctx := env.SetInCtx(context.Background())

		getBuilds := func(decompress bool) []*bbpb.Build {
			fakeData := scFake.Data()["test_namespace/build.proto"]
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
					data, err = io.ReadAll(r)
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

		Convey(`basic`, func() {
			Convey(`success`, func() {
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					build.Status = bbpb.Status_SCHEDULED
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
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					return errors.New("bad stuff")
				})
				So(exitCode, ShouldEqual, 1)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status:          bbpb.Status_FAILURE,
					SummaryMarkdown: "Final error: bad stuff",
					Output: &bbpb.Build_Output{
						Properties: &structpb.Struct{},
					},
				})
			})

			Convey(`infra failure`, func() {
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					return errors.New("bad stuff", InfraErrorTag)
				})
				So(exitCode, ShouldEqual, 1)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status:          bbpb.Status_INFRA_FAILURE,
					SummaryMarkdown: "Final infra error: bad stuff",
					Output: &bbpb.Build_Output{
						Properties: &structpb.Struct{},
					},
				})
			})

			Convey(`panic`, func() {
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					panic(errors.New("bad stuff"))
				})
				So(exitCode, ShouldEqual, 2)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status:          bbpb.Status_INFRA_FAILURE,
					SummaryMarkdown: "Final panic: bad stuff",
					Output: &bbpb.Build_Output{
						Properties: &structpb.Struct{},
					},
				})
			})

			Convey(`respect user program status`, func() {
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					build.Status = bbpb.Status_INFRA_FAILURE
					build.SummaryMarkdown = "status set inside"
					return nil
				})
				So(exitCode, ShouldEqual, 0)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status:          bbpb.Status_INFRA_FAILURE,
					SummaryMarkdown: "status set inside",
					Output: &bbpb.Build_Output{
						Properties: &structpb.Struct{},
					},
				})
			})
		})

		Convey(`send`, func() {
			exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
				build.SummaryMarkdown = "Hi. I did stuff."
				bs()
				return errors.New("oh no i failed")
			})
			So(exitCode, ShouldEqual, 1)
			builds := getBuilds(false)
			So(len(builds), ShouldEqual, 2)
			So(builds[0], ShouldResembleProto, &bbpb.Build{
				SummaryMarkdown: "Hi. I did stuff.",
				Output: &bbpb.Build_Output{
					Properties: &structpb.Struct{},
				},
			})
			So(builds[len(builds)-1], ShouldResembleProto, &bbpb.Build{
				Status:          bbpb.Status_FAILURE,
				SummaryMarkdown: "Hi. I did stuff.\n\nFinal error: oh no i failed",
				Output: &bbpb.Build_Output{
					Properties: &structpb.Struct{},
				},
			})
		})

		Convey(`send (zlib)`, func() {
			exitCode := runCtx(ctx, args, []Option{WithZlibCompression(5)}, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
				build.SummaryMarkdown = "Hi. I did stuff."
				bs()
				return errors.New("oh no i failed")
			})
			So(exitCode, ShouldEqual, 1)
			builds := getBuilds(true)
			So(len(builds), ShouldEqual, 2)
			So(builds[0], ShouldResembleProto, &bbpb.Build{
				SummaryMarkdown: "Hi. I did stuff.",
				Output: &bbpb.Build_Output{
					Properties: &structpb.Struct{},
				},
			})
			So(builds[len(builds)-1], ShouldResembleProto, &bbpb.Build{
				Status:          bbpb.Status_FAILURE,
				SummaryMarkdown: "Hi. I did stuff.\n\nFinal error: oh no i failed",
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
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					build.SummaryMarkdown = "Hi."
					err := WriteProperties(build.Output.Properties, map[string]interface{}{
						"some": "thing",
					})
					if err != nil {
						panic(err)
					}

					return nil
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
				data, err := os.ReadFile(outFile)
				So(err, ShouldBeNil)
				So(string(data), ShouldResemble,
					"`\f\x82\x01\x13\n\x11\n\x0f\n\x04some\x12\a\x1a\x05thing\xa2\x01\x03Hi.")
			})

			Convey(`textpb`, func() {
				outFile := filepath.Join(tdir, "out.textpb")
				args = append(args, luciexe.OutputCLIArg, outFile)
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					build.SummaryMarkdown = "Hi."
					return nil
				})
				So(exitCode, ShouldEqual, 0)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status:          bbpb.Status_SUCCESS,
					SummaryMarkdown: "Hi.",
					Output: &bbpb.Build_Output{
						Properties: &structpb.Struct{},
					},
				})
				data, err := os.ReadFile(outFile)
				So(err, ShouldBeNil)
				So(string(data), ShouldResemble,
					"status: SUCCESS\nsummary_markdown: \"Hi.\"\noutput: <\n  properties: <\n  >\n>\n")
			})

			Convey(`jsonpb`, func() {
				outFile := filepath.Join(tdir, "out.json")
				args = append(args, luciexe.OutputCLIArg, outFile)
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					build.SummaryMarkdown = "Hi."
					return nil
				})
				So(exitCode, ShouldEqual, 0)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status:          bbpb.Status_SUCCESS,
					SummaryMarkdown: "Hi.",
					Output: &bbpb.Build_Output{
						Properties: &structpb.Struct{},
					},
				})
				data, err := os.ReadFile(outFile)
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
					exitcode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
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
					exitcode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
						So(userArgs, ShouldResemble, expectedUserArgs)
						return nil
					})
					So(exitcode, ShouldEqual, 0)
				})
			})

			Convey(`write output on error`, func() {
				outFile := filepath.Join(tdir, "out.json")
				args = append(args, luciexe.OutputCLIArg, outFile)
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					build.SummaryMarkdown = "Hi."
					return errors.New("bad stuff")
				})
				So(exitCode, ShouldEqual, 1)
				data, err := os.ReadFile(outFile)
				So(err, ShouldBeNil)
				So(string(data), ShouldResemble,
					"{\n  \"status\": \"FAILURE\",\n  \"summary_markdown\": \"Hi.\\n\\nFinal error: bad stuff\",\n  \"output\": {\n    \"properties\": {\n      }\n  }\n}")
			})
		})
	})
}
