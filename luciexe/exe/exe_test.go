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
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/luciexe"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestExe(t *testing.T) {
	t.Parallel()

	Convey(`test exe`, t, func() {
		client := streamclient.NewFake("test_namespace")
		ldClient := &bootstrap.Bootstrap{
			CoordinatorHost: "test.example.com",
			Project:         "test_project",
			Prefix:          "test_prefix",
			Namespace:       "test_namespace",
			Client:          client.Client,
		}
		var ldErr error
		bootstrapGet := func() (*bootstrap.Bootstrap, error) { return ldClient, ldErr }

		getBuilds := func() []*bbpb.Build {
			dgs := client.GetFakeData()["test_namespace/build.proto"].GetDatagrams()
			So(len(dgs), ShouldBeGreaterThanOrEqualTo, 1)

			ret := make([]*bbpb.Build, len(dgs))
			for i, dg := range dgs {
				ret[i] = &bbpb.Build{}
				So(proto.Unmarshal([]byte(dg), ret[i]), ShouldBeNil)
			}
			return ret
		}
		lastBuild := func() *bbpb.Build {
			builds := getBuilds()
			return builds[len(builds)-1]
		}

		args := []string{"fake_test_executable"}

		ctx := context.Background()

		Convey(`basic`, func() {
			Convey(`success`, func() {
				exitCode := runCtx(ctx, &args, bootstrapGet, func(ctx context.Context, build *bbpb.Build, bs BuildSender) error {
					return nil
				})
				So(exitCode, ShouldEqual, 0)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status: bbpb.Status_SUCCESS,
				})
			})

			Convey(`failure`, func() {
				exitCode := runCtx(ctx, &args, bootstrapGet, func(ctx context.Context, build *bbpb.Build, bs BuildSender) error {
					return errors.New("bad stuff")
				})
				So(exitCode, ShouldEqual, 1)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status:          bbpb.Status_FAILURE,
					SummaryMarkdown: "Final error: bad stuff",
				})
			})

			Convey(`infra failure`, func() {
				exitCode := runCtx(ctx, &args, bootstrapGet, func(ctx context.Context, build *bbpb.Build, bs BuildSender) error {
					return errors.New("bad stuff", InfraErrorTag)
				})
				So(exitCode, ShouldEqual, 1)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status:          bbpb.Status_INFRA_FAILURE,
					SummaryMarkdown: "Final infra error: bad stuff",
				})
			})

			Convey(`panic`, func() {
				exitCode := runCtx(ctx, &args, bootstrapGet, func(ctx context.Context, build *bbpb.Build, bs BuildSender) error {
					panic(errors.New("bad stuff"))
				})
				So(exitCode, ShouldEqual, 2)
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status:          bbpb.Status_INFRA_FAILURE,
					SummaryMarkdown: "Final panic: bad stuff",
				})
			})
		})

		Convey(`send`, func() {
			exitCode := runCtx(ctx, &args, bootstrapGet, func(ctx context.Context, build *bbpb.Build, bs BuildSender) error {
				build.SummaryMarkdown = "Hi. I did stuff."
				bs()
				return errors.New("oh no i failed")
			})
			So(exitCode, ShouldEqual, 1)
			builds := getBuilds()
			So(len(builds), ShouldEqual, 2)
			So(builds[0], ShouldResembleProto, &bbpb.Build{
				SummaryMarkdown: "Hi. I did stuff.",
			})
			So(builds[len(builds)-1], ShouldResembleProto, &bbpb.Build{
				Status:          bbpb.Status_FAILURE,
				SummaryMarkdown: "Hi. I did stuff.\n\nFinal error: oh no i failed",
			})
		})

		Convey(`output`, func() {
			tdir, err := ioutil.TempDir("", "luciexe-exe-test")
			So(err, ShouldBeNil)
			defer os.RemoveAll(tdir)

			Convey(`binary`, func() {
				outFile := filepath.Join(tdir, "out.pb")
				args = append(args, luciexe.OutputCLIArg, outFile)
				exitCode := runCtx(ctx, &args, bootstrapGet, func(ctx context.Context, build *bbpb.Build, bs BuildSender) error {
					build.SummaryMarkdown = "Hi."
					return nil
				})
				So(exitCode, ShouldEqual, 0)
				So(args, ShouldResemble, []string{"fake_test_executable"})
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status:          bbpb.Status_SUCCESS,
					SummaryMarkdown: "Hi.",
				})
				data, err := ioutil.ReadFile(outFile)
				So(err, ShouldBeNil)
				So(string(data), ShouldResemble, "`\f\xa2\x01\x03Hi.")
			})

			Convey(`textpb`, func() {
				outFile := filepath.Join(tdir, "out.textpb")
				args = append(args, luciexe.OutputCLIArg, outFile)
				exitCode := runCtx(ctx, &args, bootstrapGet, func(ctx context.Context, build *bbpb.Build, bs BuildSender) error {
					build.SummaryMarkdown = "Hi."
					return nil
				})
				So(exitCode, ShouldEqual, 0)
				So(args, ShouldResemble, []string{"fake_test_executable"})
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status:          bbpb.Status_SUCCESS,
					SummaryMarkdown: "Hi.",
				})
				data, err := ioutil.ReadFile(outFile)
				So(err, ShouldBeNil)
				So(string(data), ShouldResemble,
					"status: SUCCESS\nsummary_markdown: \"Hi.\"\n")
			})

			Convey(`jsonpb`, func() {
				outFile := filepath.Join(tdir, "out.json")
				args = append(args, luciexe.OutputCLIArg, outFile)
				exitCode := runCtx(ctx, &args, bootstrapGet, func(ctx context.Context, build *bbpb.Build, bs BuildSender) error {
					build.SummaryMarkdown = "Hi."
					return nil
				})
				So(exitCode, ShouldEqual, 0)
				So(args, ShouldResemble, []string{"fake_test_executable"})
				So(lastBuild(), ShouldResembleProto, &bbpb.Build{
					Status:          bbpb.Status_SUCCESS,
					SummaryMarkdown: "Hi.",
				})
				data, err := ioutil.ReadFile(outFile)
				So(err, ShouldBeNil)
				So(string(data), ShouldResemble,
					"{\n  \"status\": \"SUCCESS\",\n  \"summary_markdown\": \"Hi.\"\n}")
			})
		})
	})
}
