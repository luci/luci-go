// Copyright 2020 The LUCI Authors.
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

package build

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/time/rate"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"

	"go.chromium.org/luci/luciexe/build/internal/testpb"
	"go.chromium.org/luci/luciexe/build/properties"
)

func init() {
	// ensure that send NEVER blocks while testing Main functionality
	mainSendRate = rate.Inf
}

func TestMain(t *testing.T) {
	// avoid t.Parallel() because this registers property handlers.

	ftt.Run(`Main`, t, func(t *ftt.Test) {
		// reset the property registry
		Properties = &properties.Registry{}
		topLevel := RegisterProperty[*testpb.TopLevel]("")

		ctx := memlogger.Use(context.Background())
		logs := logging.Get(ctx).(*memlogger.MemLogger)

		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		nowpb := timestamppb.New(testclock.TestRecentTimeUTC)

		scFake := streamclient.NewFake()
		defer scFake.Unregister()

		env := environ.New(nil)
		env.Set(bootstrap.EnvStreamServerPath, scFake.StreamServerPath())
		env.Set(bootstrap.EnvNamespace, "u")
		ctx = env.SetInCtx(ctx)

		tdir := t.TempDir()

		finalBuildPath := filepath.Join(tdir, "finalBuild.json")
		args := []string{"myprogram", "--output", finalBuildPath}
		stdin := &bytes.Buffer{}

		mkStruct := func(dictlike map[string]any) *structpb.Struct {
			s, err := structpb.NewStruct(dictlike)
			assert.Loosely(t, err, should.BeNil)
			return s
		}

		writeStdinProps := func(dictlike map[string]any) {
			b := &bbpb.Build{
				Input: &bbpb.Build_Input{
					Properties: mkStruct(dictlike),
				},
			}
			data, err := proto.Marshal(b)
			assert.Loosely(t, err, should.BeNil)
			_, err = stdin.Write(data)
			assert.Loosely(t, err, should.BeNil)
		}

		getFinal := func() *bbpb.Build {
			data, err := os.ReadFile(finalBuildPath)
			assert.Loosely(t, err, should.BeNil)
			ret := &bbpb.Build{}
			assert.Loosely(t, protojson.Unmarshal(data, ret), should.BeNil)

			// proto module is cute and tries to introduce non-deterministic
			// characters into their error messages. This is annoying and unhelpful
			// for tests where error messages intentionally can show up in the Build
			// output. We manually normalize them here. Replaces non-breaking space
			// (U+00a0) with space (U+0020)
			ret.SummaryMarkdown = strings.ReplaceAll(ret.SummaryMarkdown, " ", " ")

			return ret
		}

		t.Run(`good`, func(t *ftt.Test) {
			t.Run(`simple`, func(t *ftt.Test) {
				err := main(ctx, args, stdin, func(ctx context.Context, args []string, st *State) error {
					assert.Loosely(t, args, should.BeNil)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, getFinal(), should.Resemble(&bbpb.Build{
					StartTime: nowpb,
					EndTime:   nowpb,
					Status:    bbpb.Status_SUCCESS,
					Output: &bbpb.Build_Output{
						Status: bbpb.Status_SUCCESS,
					},
					Input: &bbpb.Build_Input{},
				}))
			})

			t.Run(`user args`, func(t *ftt.Test) {
				args = append(args, "--", "custom", "stuff")
				err := main(ctx, args, stdin, func(ctx context.Context, args []string, st *State) error {
					assert.Loosely(t, args, should.Resemble([]string{"custom", "stuff"}))
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, getFinal(), should.Resemble(&bbpb.Build{
					StartTime: nowpb,
					EndTime:   nowpb,
					Status:    bbpb.Status_SUCCESS,
					Output: &bbpb.Build_Output{
						Status: bbpb.Status_SUCCESS,
					},
					Input: &bbpb.Build_Input{},
				}))
			})

			t.Run(`inputProps`, func(t *ftt.Test) {
				writeStdinProps(map[string]any{
					"field": "something",
					"$cool": "blah",
				})

				err := main(ctx, args, stdin, func(ctx context.Context, args []string, st *State) error {
					assert.Loosely(t, topLevel.GetInput(ctx), should.Resemble(&testpb.TopLevel{
						Field:         "something",
						JsonNameField: "blah",
					}))
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run(`help`, func(t *ftt.Test) {
				args = append(args, "--help")
				err := main(ctx, args, stdin, func(ctx context.Context, args []string, st *State) error {
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "`myprogram` is a `luciexe` binary. See go.chromium.org/luci/luciexe."))
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "======= I/O Proto ======="))
				// TODO(iannucci): check I/O proto when implemented
			})
		})

		t.Run(`errors`, func(t *ftt.Test) {
			t.Run(`returned`, func(t *ftt.Test) {
				err := main(ctx, args, stdin, func(ctx context.Context, args []string, st *State) error {
					assert.Loosely(t, args, should.BeNil)
					return errors.New("bad stuff")
				})
				assert.Loosely(t, err, should.Equal(errNonSuccess))
				assert.Loosely(t, getFinal(), should.Resemble(&bbpb.Build{
					StartTime: nowpb,
					EndTime:   nowpb,
					Status:    bbpb.Status_FAILURE,
					Output: &bbpb.Build_Output{
						Status: bbpb.Status_FAILURE,
					},
					Input: &bbpb.Build_Input{},
				}))
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Error, "set status: FAILURE: bad stuff"))
			})

			t.Run(`panic`, func(t *ftt.Test) {
				err := main(ctx, args, stdin, func(ctx context.Context, args []string, st *State) error {
					assert.Loosely(t, args, should.BeNil)
					panic("BAD THINGS")
				})
				assert.Loosely(t, err, should.Equal(errNonSuccess))
				assert.Loosely(t, getFinal(), should.Resemble(&bbpb.Build{
					StartTime: nowpb,
					EndTime:   nowpb,
					Status:    bbpb.Status_INFRA_FAILURE,
					Output: &bbpb.Build_Output{
						Status: bbpb.Status_INFRA_FAILURE,
					},
					Input: &bbpb.Build_Input{},
				}))
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Error, "set status: INFRA_FAILURE: PANIC"))
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Error, "recovered panic: BAD THINGS"))
			})

			t.Run(`inputProps`, func(t *ftt.Test) {
				writeStdinProps(map[string]any{
					"field": 100,
				})

				err := main(ctx, args, stdin, func(ctx context.Context, args []string, st *State) error {
					return nil
				})
				assert.Loosely(t, err, should.ErrLike(`invalid value for string field field: 100`))
				summary := "fatal error starting build: build.Start: properties.Registry.Instantiate - input[top-level]: protoFromStruct[*testpb.TopLevel]: proto: (line 1:10): invalid value for string field field: 100"
				final := getFinal()
				// protobuf package deliberately introduce random prefix:
				// https://github.com/protocolbuffers/protobuf-go/blob/master/internal/errors/errors.go#L26
				assert.Loosely(t, strings.ReplaceAll(final.SummaryMarkdown, "\u00a0", " "), should.Equal(summary))
				assert.Loosely(t, strings.ReplaceAll(final.Output.SummaryMarkdown, "\u00a0", " "), should.Equal(summary))
				assert.Loosely(t, final.Status, should.Equal(bbpb.Status_INFRA_FAILURE))
				assert.Loosely(t, final.Output.Status, should.Equal(bbpb.Status_INFRA_FAILURE))
			})

		})
	})
}
