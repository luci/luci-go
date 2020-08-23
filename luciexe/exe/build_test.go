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

package exe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	structpb "github.com/golang/protobuf/ptypes/struct"
	"golang.org/x/time/rate"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/luciexe/exe/proptools"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBuild(t *testing.T) {
	t.Parallel()

	Convey(`test build`, t, func() {
		client := streamclient.NewFake("u")
		var lastBuild *bbpb.Build

		ctx := context.Background()

		ctx, build := SinkBuildUpdates(ctx, &bbpb.Build{}, client.Client, rate.Inf, func(b *bbpb.Build) {
			lastBuild = b
		})

		Convey(`basic modify`, func() {
			err := build.Modify(func(b *BuildView) error {
				b.SummaryMarkdown = "hi"
				return nil
			})
			So(err, ShouldBeNil)
			build.Detach(ctx)

			So(lastBuild.SummaryMarkdown, ShouldEqual, "hi")
		})

		Convey(`parallel modify`, func() {
			expectedVals := stringset.New(100)
			var wg sync.WaitGroup
			for i := 0; i < 100; i++ {
				s := fmt.Sprintf("%d", i)
				expectedVals.Add(s)
				wg.Add(1)
				go func() {
					defer wg.Done()
					build.Modify(func(b *BuildView) error {
						b.SummaryMarkdown += s + "\n"
						return nil
					})
				}()
			}
			wg.Wait()
			build.Detach(ctx)

			actualVals := stringset.NewFromSlice(strings.Split(lastBuild.SummaryMarkdown, "\n")...)
			actualVals.Del("")

			So(actualVals, ShouldResemble, expectedVals)
		})

		Convey(`modify properties`, func() {
			err := ModifyProperties(ctx, func(props *structpb.Struct) error {
				return proptools.WriteProperties(props, map[string]interface{}{
					"cool": "stuff",
				})
			})
			So(err, ShouldBeNil)

			build.Detach(ctx)

			So(lastBuild.Output.Properties, ShouldResembleProto, &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"cool": {Kind: &structpb.Value_StringValue{StringValue: "stuff"}},
				},
			})
		})

		Convey(`namespace properties`, func() {
			doCoolStuff := func(ctx context.Context) {
				err := ModifyProperties(ctx, func(props *structpb.Struct) error {
					return proptools.WriteProperties(props, map[string]interface{}{
						"cool": "stuff",
					})
				})
				So(err, ShouldBeNil)
			}

			doCoolStuff(NamespaceProperties(ctx, "ns1"))
			doCoolStuff(NamespaceProperties(ctx, "ns2"))

			build.Detach(ctx)

			So(lastBuild.Output.Properties, ShouldResembleProto, &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"ns1": {Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"cool": {Kind: &structpb.Value_StringValue{StringValue: "stuff"}},
						},
					}}},
					"ns2": {Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"cool": {Kind: &structpb.Value_StringValue{StringValue: "stuff"}},
						},
					}}},
				},
			})
		})

		Convey(`overwrite properties`, func() {
			doCoolStuff := func(ctx context.Context) {
				err := ModifyProperties(ctx, func(props *structpb.Struct) error {
					return proptools.WriteProperties(props, map[string]interface{}{
						"cool": "stuff",
					})
				})
				So(err, ShouldBeNil)
			}

			doCoolStuff(ctx)                              // cool: stuff
			doCoolStuff(NamespaceProperties(ctx, "cool")) // overwrites cool

			build.Detach(ctx)

			So(lastBuild.Output.Properties, ShouldResembleProto, &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"cool": {Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"cool": {Kind: &structpb.Value_StringValue{StringValue: "stuff"}},
						},
					}}},
				},
			})
		})

		Convey(`detach`, func() {
			buildMsg, send := build.Detach(ctx)
			buildMsg2, _ := build.Detach(ctx)
			So(buildMsg, ShouldEqual, buildMsg2)

			So(buildMsg, ShouldResemble, &bbpb.Build{Output: &bbpb.Build_Output{}})
			buildMsg.SummaryMarkdown = "narf"

			So(lastBuild.GetSummaryMarkdown(), ShouldResemble, "")
			send()
			So(lastBuild.GetSummaryMarkdown(), ShouldResemble, "narf")

			// Modify has no effect.
			err := build.Modify(func(bv *BuildView) error {
				bv.Critical = bbpb.Trinary_YES
				return nil
			})
			So(err, ShouldEqual, ErrBuildDetached)
			send()

			So(lastBuild.GetCritical(), ShouldResemble, bbpb.Trinary_UNSET)
		})

		Convey(`logs`, func() {
			Convey(`text`, func() {
				l, err := build.Log(ctx, "cool_log")
				So(err, ShouldBeNil)

				fmt.Fprintf(l, "this is neat!\n")
				fmt.Fprintf(l, "with some lines\n")
				l.Close()

				So(client.GetFakeData()["u/l/cool_log"].GetStreamData(),
					ShouldResemble, "this is neat!\nwith some lines\n")

				build.Detach(ctx)

				So(lastBuild.Output.Logs[0], ShouldResembleProto, &bbpb.Log{
					Name: "cool_log",
					Url:  "l/cool_log",
				})
			})

			Convey(`binary`, func() {
				l, err := build.LogBinary(ctx, "cool_log")
				So(err, ShouldBeNil)

				fmt.Fprintf(l, "this is neat!\n")
				fmt.Fprintf(l, "with some lines\n")
				l.Close()

				So(client.GetFakeData()["u/l/cool_log"].GetStreamData(),
					ShouldResemble, "this is neat!\nwith some lines\n")

				build.Detach(ctx)

				So(lastBuild.Output.Logs[0], ShouldResembleProto, &bbpb.Log{
					Name: "cool_log",
					Url:  "l/cool_log",
				})
			})

			Convey(`file`, func() {
				fname := filepath.Join(t.TempDir(), "some_file")
				f, err := os.Create(fname)
				So(err, ShouldBeNil)
				fmt.Fprintf(f, "this is neat!\n")
				fmt.Fprintf(f, "with some lines\n")
				So(f.Close(), ShouldBeNil)

				So(build.LogFile(ctx, "a log", fname), ShouldBeNil)

				So(client.GetFakeData()["u/l/0"].GetStreamData(),
					ShouldResemble, "this is neat!\nwith some lines\n")

				build.Detach(ctx)

				So(lastBuild.Output.Logs[0], ShouldResembleProto, &bbpb.Log{
					Name: "a log",
					Url:  "l/0",
				})
			})

			Convey(`datagram`, func() {
				l, err := build.LogDatagram(ctx, "dgram")
				So(err, ShouldBeNil)

				l.WriteDatagram([]byte("this is neat!"))
				l.WriteDatagram([]byte("with some datagrams"))
				l.Close()

				So(client.GetFakeData()["u/l/dgram"].GetDatagrams(), ShouldResemble, []string{
					"this is neat!",
					"with some datagrams",
				})

				build.Detach(ctx)

				So(lastBuild.Output.Logs[0], ShouldResembleProto, &bbpb.Log{
					Name: "dgram",
					Url:  "l/dgram",
				})
			})

		})

	})

	Convey(`test nil build`, t, func() {
		ptime := google.NewTimestamp(testclock.TestTimeUTC)

		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, testclock.TestTimeUTC)

		ctx, buildObj := SinkBuildUpdates(ctx, &bbpb.Build{}, nil, 0, nil)

		buildObj.Modify(func(bv *BuildView) error {
			bv.SummaryMarkdown = "hello"
			return nil
		})

		lf, err := buildObj.Log(ctx, "extra_log")
		So(err, ShouldBeNil)

		_, err = lf.Write([]byte("this goes nowhere"))
		So(err, ShouldBeNil)
		So(lf.Close(), ShouldBeNil)

		WithStep(ctx, "some name", func(ctx context.Context, s *Step) error {
			lf, err := s.Log(ctx, "extra_log")
			So(err, ShouldBeNil)

			_, err = lf.Write([]byte("this goes nowhere"))
			So(err, ShouldBeNil)
			So(lf.Close(), ShouldBeNil)

			logging.Infof(ctx, "ignored")

			return s.Modify(ctx, func(sv *StepView) error {
				sv.SummaryMarkdown = "herp derp"
				return nil
			})
		})

		build, _ := buildObj.Detach(ctx)
		So(build, ShouldResembleProto, &bbpb.Build{
			Output: &bbpb.Build_Output{
				Logs: []*bbpb.Log{
					{Name: "extra_log", Url: "l/extra_log"},
				},
			},
			SummaryMarkdown: "hello",
			Steps: []*bbpb.Step{
				{
					Name:            "some name",
					SummaryMarkdown: "herp derp",
					StartTime:       ptime,
					EndTime:         ptime,
					Status:          bbpb.Status_SUCCESS,
					Logs: []*bbpb.Log{
						{Name: "extra_log", Url: "s/0/l/extra_log"},
						{Name: "log", Url: "s/0/l/log"},
					},
				},
			},
		})

	})
}
