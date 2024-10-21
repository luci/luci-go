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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/luciexe"
)

func TestExe(t *testing.T) {
	t.Parallel()

	ftt.Run(`test exe`, t, func(t *ftt.Test) {
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
				assert.Loosely(t, fakeData.GetFlags().ContentType, should.Equal(luciexe.BuildProtoZlibContentType))
			} else {
				assert.Loosely(t, fakeData.GetFlags().ContentType, should.Equal(luciexe.BuildProtoContentType))
			}

			dgs := fakeData.GetDatagrams()
			assert.Loosely(t, len(dgs), should.BeGreaterThanOrEqual(1))

			ret := make([]*bbpb.Build, len(dgs))
			for i, dg := range dgs {
				ret[i] = &bbpb.Build{}

				var data []byte

				if decompress {
					r, err := zlib.NewReader(bytes.NewBufferString(dg))
					assert.Loosely(t, err, should.BeNil)
					data, err = io.ReadAll(r)
					assert.Loosely(t, err, should.BeNil)
				} else {
					data = []byte(dg)
				}

				assert.Loosely(t, proto.Unmarshal(data, ret[i]), should.BeNil)
			}
			return ret
		}
		lastBuild := func() *bbpb.Build {
			builds := getBuilds(false)
			return builds[len(builds)-1]
		}

		args := []string{"fake_test_executable"}

		t.Run(`basic`, func(t *ftt.Test) {
			t.Run(`success`, func(t *ftt.Test) {
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					build.Status = bbpb.Status_SCHEDULED
					return nil
				})
				assert.Loosely(t, exitCode, should.BeZero)
				assert.Loosely(t, lastBuild(), should.Resemble(&bbpb.Build{
					Status: bbpb.Status_SUCCESS,
					Output: &bbpb.Build_Output{
						Properties: &structpb.Struct{},
						Status:     bbpb.Status_SUCCESS,
					},
				}))
			})

			t.Run(`failure`, func(t *ftt.Test) {
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					return errors.New("bad stuff")
				})
				assert.Loosely(t, exitCode, should.Equal(1))
				assert.Loosely(t, lastBuild(), should.Resemble(&bbpb.Build{
					Status:          bbpb.Status_FAILURE,
					SummaryMarkdown: "Final error: bad stuff",
					Output: &bbpb.Build_Output{
						Properties:      &structpb.Struct{},
						Status:          bbpb.Status_FAILURE,
						SummaryMarkdown: "Final error: bad stuff",
					},
				}))
			})

			t.Run(`infra failure`, func(t *ftt.Test) {
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					return errors.New("bad stuff", InfraErrorTag)
				})
				assert.Loosely(t, exitCode, should.Equal(1))
				assert.Loosely(t, lastBuild(), should.Resemble(&bbpb.Build{
					Status:          bbpb.Status_INFRA_FAILURE,
					SummaryMarkdown: "Final infra error: bad stuff",
					Output: &bbpb.Build_Output{
						Properties:      &structpb.Struct{},
						Status:          bbpb.Status_INFRA_FAILURE,
						SummaryMarkdown: "Final infra error: bad stuff",
					},
				}))
			})

			t.Run(`panic`, func(t *ftt.Test) {
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					panic(errors.New("bad stuff"))
				})
				assert.Loosely(t, exitCode, should.Equal(2))
				assert.Loosely(t, lastBuild(), should.Resemble(&bbpb.Build{
					Status:          bbpb.Status_INFRA_FAILURE,
					SummaryMarkdown: "Final panic: bad stuff",
					Output: &bbpb.Build_Output{
						Properties:      &structpb.Struct{},
						Status:          bbpb.Status_INFRA_FAILURE,
						SummaryMarkdown: "Final panic: bad stuff",
					},
				}))
			})

			t.Run(`respect user program status`, func(t *ftt.Test) {
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					build.Status = bbpb.Status_INFRA_FAILURE
					build.SummaryMarkdown = "status set inside"
					return nil
				})
				assert.Loosely(t, exitCode, should.BeZero)
				assert.Loosely(t, lastBuild(), should.Resemble(&bbpb.Build{
					Status:          bbpb.Status_INFRA_FAILURE,
					SummaryMarkdown: "status set inside",
					Output: &bbpb.Build_Output{
						Properties:      &structpb.Struct{},
						Status:          bbpb.Status_INFRA_FAILURE,
						SummaryMarkdown: "status set inside",
					},
				}))
			})
		})

		t.Run(`send`, func(t *ftt.Test) {
			exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
				build.SummaryMarkdown = "Hi. I did stuff."
				bs()
				return errors.New("oh no i failed")
			})
			assert.Loosely(t, exitCode, should.Equal(1))
			builds := getBuilds(false)
			assert.Loosely(t, len(builds), should.Equal(2))
			assert.Loosely(t, builds[0], should.Resemble(&bbpb.Build{
				SummaryMarkdown: "Hi. I did stuff.",
				Output: &bbpb.Build_Output{
					Properties: &structpb.Struct{},
				},
			}))
			assert.Loosely(t, builds[len(builds)-1], should.Resemble(&bbpb.Build{
				Status:          bbpb.Status_FAILURE,
				SummaryMarkdown: "Hi. I did stuff.\n\nFinal error: oh no i failed",
				Output: &bbpb.Build_Output{
					Properties:      &structpb.Struct{},
					Status:          bbpb.Status_FAILURE,
					SummaryMarkdown: "Hi. I did stuff.\n\nFinal error: oh no i failed",
				},
			}))
		})

		t.Run(`send (zlib)`, func(t *ftt.Test) {
			exitCode := runCtx(ctx, args, []Option{WithZlibCompression(5)}, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
				build.SummaryMarkdown = "Hi. I did stuff."
				bs()
				return errors.New("oh no i failed")
			})
			assert.Loosely(t, exitCode, should.Equal(1))
			builds := getBuilds(true)
			assert.Loosely(t, len(builds), should.Equal(2))
			assert.Loosely(t, builds[0], should.Resemble(&bbpb.Build{
				SummaryMarkdown: "Hi. I did stuff.",
				Output: &bbpb.Build_Output{
					Properties: &structpb.Struct{},
				},
			}))
			assert.Loosely(t, builds[len(builds)-1], should.Resemble(&bbpb.Build{
				Status:          bbpb.Status_FAILURE,
				SummaryMarkdown: "Hi. I did stuff.\n\nFinal error: oh no i failed",
				Output: &bbpb.Build_Output{
					Properties:      &structpb.Struct{},
					Status:          bbpb.Status_FAILURE,
					SummaryMarkdown: "Hi. I did stuff.\n\nFinal error: oh no i failed",
				},
			}))
		})

		t.Run(`output`, func(t *ftt.Test) {
			tdir, err := ioutil.TempDir("", "luciexe-exe-test")
			assert.Loosely(t, err, should.BeNil)
			defer os.RemoveAll(tdir)

			t.Run(`binary`, func(t *ftt.Test) {
				outFile := filepath.Join(tdir, "out.pb")
				args = append(args, luciexe.OutputCLIArg, outFile)
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					build.SummaryMarkdown = "Hi."
					err := WriteProperties(build.Output.Properties, map[string]any{
						"some": "thing",
					})
					if err != nil {
						panic(err)
					}

					return nil
				})
				assert.Loosely(t, exitCode, should.BeZero)
				assert.Loosely(t, lastBuild(), should.Resemble(&bbpb.Build{
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
						Status:          bbpb.Status_SUCCESS,
						SummaryMarkdown: "Hi.",
					},
				}))
				data, err := os.ReadFile(outFile)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(data), should.Match(
					"`\f\x82\x01\x1a\n\x11\n\x0f\n\x04some\x12\a\x1a\x05thing\x12\x03Hi.0\f\xa2\x01\x03Hi."))
			})

			t.Run(`textpb`, func(t *ftt.Test) {
				outFile := filepath.Join(tdir, "out.textpb")
				args = append(args, luciexe.OutputCLIArg, outFile)
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					build.SummaryMarkdown = "Hi."
					return nil
				})
				assert.Loosely(t, exitCode, should.BeZero)
				assert.Loosely(t, lastBuild(), should.Resemble(&bbpb.Build{
					Status:          bbpb.Status_SUCCESS,
					SummaryMarkdown: "Hi.",
					Output: &bbpb.Build_Output{
						Properties:      &structpb.Struct{},
						Status:          bbpb.Status_SUCCESS,
						SummaryMarkdown: "Hi.",
					},
				}))
				data, err := os.ReadFile(outFile)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(data), should.Match(
					"status: SUCCESS\nsummary_markdown: \"Hi.\"\noutput: <\n  properties: <\n  >\n  status: SUCCESS\n  summary_markdown: \"Hi.\"\n>\n"))
			})

			t.Run(`jsonpb`, func(t *ftt.Test) {
				outFile := filepath.Join(tdir, "out.json")
				args = append(args, luciexe.OutputCLIArg, outFile)
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					build.SummaryMarkdown = "Hi."
					return nil
				})
				assert.Loosely(t, exitCode, should.BeZero)
				assert.Loosely(t, lastBuild(), should.Resemble(&bbpb.Build{
					Status:          bbpb.Status_SUCCESS,
					SummaryMarkdown: "Hi.",
					Output: &bbpb.Build_Output{
						Properties:      &structpb.Struct{},
						Status:          bbpb.Status_SUCCESS,
						SummaryMarkdown: "Hi.",
					},
				}))
				data, err := os.ReadFile(outFile)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(data), should.Match(
					"{\n  \"status\": \"SUCCESS\",\n  \"summary_markdown\": \"Hi.\",\n  \"output\": {\n    \"properties\": {\n      },\n    \"status\": \"SUCCESS\",\n    \"summary_markdown\": \"Hi.\"\n  }\n}"))
			})

			t.Run(`pass through user args`, func(t *ftt.Test) {
				// Delimiter inside user args should also be passed through
				expectedUserArgs := []string{"foo", "bar", ArgsDelim, "baz"}
				t.Run(`when output is not specified`, func(t *ftt.Test) {
					args = append(args, ArgsDelim)
					args = append(args, expectedUserArgs...)
					exitcode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
						assert.Loosely(t, userArgs, should.Resemble(expectedUserArgs))
						return nil
					})
					assert.Loosely(t, exitcode, should.BeZero)
				})
				t.Run(`when output is specified`, func(t *ftt.Test) {
					tdir, err := ioutil.TempDir("", "luciexe-exe-test")
					assert.Loosely(t, err, should.BeNil)
					defer os.RemoveAll(tdir)
					args = append(args, luciexe.OutputCLIArg, filepath.Join(tdir, "out.pb"), ArgsDelim)
					args = append(args, expectedUserArgs...)
					exitcode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
						assert.Loosely(t, userArgs, should.Resemble(expectedUserArgs))
						return nil
					})
					assert.Loosely(t, exitcode, should.BeZero)
				})
			})

			t.Run(`write output on error`, func(t *ftt.Test) {
				outFile := filepath.Join(tdir, "out.json")
				args = append(args, luciexe.OutputCLIArg, outFile)
				exitCode := runCtx(ctx, args, nil, func(ctx context.Context, build *bbpb.Build, userArgs []string, bs BuildSender) error {
					build.SummaryMarkdown = "Hi."
					return errors.New("bad stuff")
				})
				assert.Loosely(t, exitCode, should.Equal(1))
				data, err := os.ReadFile(outFile)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(data), should.Match(
					"{\n  \"status\": \"FAILURE\",\n  \"summary_markdown\": \"Hi.\\n\\nFinal error: bad stuff\",\n  \"output\": {\n    \"properties\": {\n      },\n    \"status\": \"FAILURE\",\n    \"summary_markdown\": \"Hi.\\n\\nFinal error: bad stuff\"\n  }\n}"))
			})
		})
	})
}
