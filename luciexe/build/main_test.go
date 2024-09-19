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
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"

	"go.chromium.org/luci/luciexe/build/internal/testpb"
	"go.chromium.org/luci/luciexe/build/properties"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func init() {
	// ensure that send NEVER blocks while testing Main functionality
	mainSendRate = rate.Inf
}

func TestMain(t *testing.T) {
	// avoid t.Parallel() because this registers property handlers.

	Convey(`Main`, t, func() {
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
			So(err, ShouldBeNil)
			return s
		}

		writeStdinProps := func(dictlike map[string]any) {
			b := &bbpb.Build{
				Input: &bbpb.Build_Input{
					Properties: mkStruct(dictlike),
				},
			}
			data, err := proto.Marshal(b)
			So(err, ShouldBeNil)
			_, err = stdin.Write(data)
			So(err, ShouldBeNil)
		}

		getFinal := func() *bbpb.Build {
			data, err := os.ReadFile(finalBuildPath)
			So(err, ShouldBeNil)
			ret := &bbpb.Build{}
			So(protojson.Unmarshal(data, ret), ShouldBeNil)

			// proto module is cute and tries to introduce non-deterministic
			// characters into their error messages. This is annoying and unhelpful
			// for tests where error messages intentionally can show up in the Build
			// output. We manually normalize them here. Replaces non-breaking space
			// (U+00a0) with space (U+0020)
			ret.SummaryMarkdown = strings.ReplaceAll(ret.SummaryMarkdown, " ", " ")

			return ret
		}

		Convey(`good`, func() {
			Convey(`simple`, func() {
				err := main(ctx, args, stdin, func(ctx context.Context, args []string, st *State) error {
					So(args, ShouldBeNil)
					return nil
				})
				So(err, ShouldBeNil)
				So(getFinal(), ShouldResembleProto, &bbpb.Build{
					StartTime: nowpb,
					EndTime:   nowpb,
					Status:    bbpb.Status_SUCCESS,
					Output: &bbpb.Build_Output{
						Status: bbpb.Status_SUCCESS,
					},
					Input: &bbpb.Build_Input{},
				})
			})

			Convey(`user args`, func() {
				args = append(args, "--", "custom", "stuff")
				err := main(ctx, args, stdin, func(ctx context.Context, args []string, st *State) error {
					So(args, ShouldResemble, []string{"custom", "stuff"})
					return nil
				})
				So(err, ShouldBeNil)
				So(getFinal(), ShouldResembleProto, &bbpb.Build{
					StartTime: nowpb,
					EndTime:   nowpb,
					Status:    bbpb.Status_SUCCESS,
					Output: &bbpb.Build_Output{
						Status: bbpb.Status_SUCCESS,
					},
					Input: &bbpb.Build_Input{},
				})
			})

			Convey(`inputProps`, func() {
				writeStdinProps(map[string]any{
					"field": "something",
					"$cool": "blah",
				})

				err := main(ctx, args, stdin, func(ctx context.Context, args []string, st *State) error {
					So(topLevel.GetInput(ctx), ShouldResembleProto, &testpb.TopLevel{
						Field:         "something",
						JsonNameField: "blah",
					})
					return nil
				})
				So(err, ShouldBeNil)
			})

			Convey(`help`, func() {
				args = append(args, "--help")
				err := main(ctx, args, stdin, func(ctx context.Context, args []string, st *State) error {
					return nil
				})
				So(err, ShouldBeNil)
				So(logs, memlogger.ShouldHaveLog, logging.Info, "`myprogram` is a `luciexe` binary. See go.chromium.org/luci/luciexe.")
				So(logs, memlogger.ShouldHaveLog, logging.Info, "======= I/O Proto =======")
				// TODO(iannucci): check I/O proto when implemented
			})
		})

		Convey(`errors`, func() {
			Convey(`returned`, func() {
				err := main(ctx, args, stdin, func(ctx context.Context, args []string, st *State) error {
					So(args, ShouldBeNil)
					return errors.New("bad stuff")
				})
				So(err, ShouldEqual, errNonSuccess)
				So(getFinal(), ShouldResembleProto, &bbpb.Build{
					StartTime: nowpb,
					EndTime:   nowpb,
					Status:    bbpb.Status_FAILURE,
					Output: &bbpb.Build_Output{
						Status: bbpb.Status_FAILURE,
					},
					Input: &bbpb.Build_Input{},
				})
				So(logs, memlogger.ShouldHaveLog, logging.Error, "set status: FAILURE: bad stuff")
			})

			Convey(`panic`, func() {
				err := main(ctx, args, stdin, func(ctx context.Context, args []string, st *State) error {
					So(args, ShouldBeNil)
					panic("BAD THINGS")
				})
				So(err, ShouldEqual, errNonSuccess)
				So(getFinal(), ShouldResembleProto, &bbpb.Build{
					StartTime: nowpb,
					EndTime:   nowpb,
					Status:    bbpb.Status_INFRA_FAILURE,
					Output: &bbpb.Build_Output{
						Status: bbpb.Status_INFRA_FAILURE,
					},
					Input: &bbpb.Build_Input{},
				})
				So(logs, memlogger.ShouldHaveLog, logging.Error, "set status: INFRA_FAILURE: PANIC")
				So(logs, memlogger.ShouldHaveLog, logging.Error, "recovered panic: BAD THINGS")
			})

			Convey(`inputProps`, func() {
				writeStdinProps(map[string]any{
					"bogus": "something",
				})

				err := main(ctx, args, stdin, func(ctx context.Context, args []string, st *State) error {
					return nil
				})
				So(err, ShouldErrLike, `unknown field "bogus"`)
				summary := "fatal error starting build: build.Start: Registry.Initialize[top-level]: protoFromStruct[*testpb.TopLevel]: proto: (line 1:2): unknown field \"bogus\""
				final := getFinal()
				// protobuf package deliberately introduce random prefix:
				// https://github.com/protocolbuffers/protobuf-go/blob/master/internal/errors/errors.go#L26
				So(strings.ReplaceAll(final.SummaryMarkdown, "\u00a0", " "), ShouldEqual, summary)
				So(strings.ReplaceAll(final.Output.SummaryMarkdown, "\u00a0", " "), ShouldEqual, summary)
				So(final.Status, ShouldEqual, bbpb.Status_INFRA_FAILURE)
				So(final.Output.Status, ShouldEqual, bbpb.Status_INFRA_FAILURE)
			})

		})
	})
}
