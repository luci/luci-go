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

package sink

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func check(ctx context.Context, cfg ServerConfig, tr *sinkpb.TestResult, expected *pb.TestResult) {
	sink, err := newSinkServer(ctx, cfg)
	sink.(*sinkpb.DecoratedSink).Service.(*sinkServer).resultIDBase = "foo"
	sink.(*sinkpb.DecoratedSink).Service.(*sinkServer).resultCounter = 100
	So(err, ShouldBeNil)

	req := &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}}
	_, err = sink.ReportTestResults(ctx, req)
	So(err, ShouldBeNil)

	cfg.Recorder.(*pb.MockRecorderClient).EXPECT().BatchCreateTestResults(
		gomock.Any(), matchBatchCreateTestResultsRequest(cfg.Invocation, expected))

	// close and drain the server to enforce all the requests processed.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	closeSinkServer(ctx, sink)
}

func TestReportTestResults(t *testing.T) {
	t.Parallel()

	ctx := metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs(AuthTokenKey, authTokenValue("secret")))

	Convey("ReportTestResults", t, func() {
		ctl := gomock.NewController(t)
		defer ctl.Finish()

		cfg := testServerConfig(ctl, "", "secret")
		tr, cleanup := validTestResult()
		defer cleanup()

		expected := &pb.TestResult{
			TestId:      tr.TestId,
			ResultId:    tr.ResultId,
			Expected:    tr.Expected,
			Status:      tr.Status,
			SummaryHtml: tr.SummaryHtml,
			StartTime:   tr.StartTime,
			Duration:    tr.Duration,
			Tags:        tr.Tags,
			TestLocation: &pb.TestLocation{
				FileName: tr.TestLocation.FileName,
			},
			TestMetadata: &pb.TestMetadata{
				Name: "name",
				Location: &pb.TestLocation{
					Repo:     "https://chromium.googlesource.com/chromium/src",
					FileName: tr.TestLocation.FileName,
				},
			},
		}
		Convey("works", func() {
			Convey("with ServerConfig.TestIDPrefix", func() {
				cfg.TestIDPrefix = "ninja://foo/bar/"
				tr.TestId = "HelloWorld.TestA"
				expected.TestId = "ninja://foo/bar/HelloWorld.TestA"
				check(ctx, cfg, tr, expected)
			})

			Convey("with ServerConfig.BaseVariant", func() {
				base := []string{"bucket", "try", "builder", "linux-rel"}
				cfg.BaseVariant = pbutil.Variant(base...)
				expected.Variant = pbutil.Variant(base...)
				check(ctx, cfg, tr, expected)
			})

			Convey("with ServerConfig.BaseTags", func() {
				t1, t2 := pbutil.StringPairs("t1", "v1"), pbutil.StringPairs("t2", "v2")
				// (nil, nil)
				cfg.BaseTags, tr.Tags, expected.Tags = nil, nil, nil
				check(ctx, cfg, tr, expected)

				// (tag, nil)
				cfg.BaseTags, tr.Tags, expected.Tags = t1, nil, t1
				check(ctx, cfg, tr, expected)

				// (nil, tag)
				cfg.BaseTags, tr.Tags, expected.Tags = nil, t1, t1
				check(ctx, cfg, tr, expected)

				// (tag1, tag2)
				cfg.BaseTags, tr.Tags, expected.Tags = t1, t2, append(t1, t2...)
				check(ctx, cfg, tr, expected)
			})
		})

		Convey("generates a random ResultID, if omitted", func() {
			tr.ResultId = ""
			expected.ResultId = "foo-00101"
			check(ctx, cfg, tr, expected)
		})

		Convey("duration", func() {
			Convey("with CoerceNegativeDuration", func() {
				cfg.CoerceNegativeDuration = true

				// duration == nil
				tr.Duration, expected.Duration = nil, nil
				check(ctx, cfg, tr, expected)

				// duration == 0
				tr.Duration, expected.Duration = ptypes.DurationProto(0), ptypes.DurationProto(0)
				check(ctx, cfg, tr, expected)

				// duration > 0
				tr.Duration, expected.Duration = ptypes.DurationProto(8), ptypes.DurationProto(8)
				check(ctx, cfg, tr, expected)

				// duration < 0
				tr.Duration = ptypes.DurationProto(-8)
				expected.Duration = ptypes.DurationProto(0)
				check(ctx, cfg, tr, expected)
			})
			Convey("without CoerceNegativeDuration", func() {
				// duration < 0
				tr.Duration = ptypes.DurationProto(-8)
				sink, err := newSinkServer(ctx, cfg)
				So(err, ShouldBeNil)

				req := &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}}
				_, err = sink.ReportTestResults(ctx, req)
				So(err, ShouldErrLike, "bad request: duration: is < 0")
			})
		})
		Convey("with ServerConfig.TestLocationBase", func() {
			cfg.TestLocationBase = "//base/"
			tr.TestLocation.FileName = "artifact_dir/a_test.cc"
			tr.TestMetadata = &pb.TestMetadata{
				Location: &pb.TestLocation{
					Repo:     "https://chromium.googlesource.com/chromium/src",
					FileName: "artifact_dir/a_test.cc",
				},
			}
			expected.TestLocation.FileName = "//base/artifact_dir/a_test.cc"
			expected.TestMetadata = &pb.TestMetadata{
				Location: &pb.TestLocation{
					Repo:     "https://chromium.googlesource.com/chromium/src",
					FileName: "//base/artifact_dir/a_test.cc",
				},
			}
			check(ctx, cfg, tr, expected)
		})

		Convey("with ServerConfig.LocationTags", func() {
			cfg.LocationTags = &sinkpb.LocationTags{
				Repos: map[string]*sinkpb.LocationTags_Repo{
					"https://chromium.googlesource.com/chromium/src": {
						Dirs: map[string]*sinkpb.LocationTags_Dir{
							"artifact_dir": {
								Tags: map[string]string{
									"monorail_project":   "chromium",
									"monorail_component": "Monorail>Component",
									"teamEmail":          "team_email@chromium.org",
									"os":                 "WINDOWS",
								},
							},
						},
					},
				},
			}
			tr.TestMetadata = &pb.TestMetadata{
				Name: "name",
				Location: &pb.TestLocation{
					Repo:     "https://chromium.googlesource.com/chromium/src",
					FileName: "//artifact_dir/a_test.cc",
				},
			}
			expected.Tags = pbutil.StringPairs(
				"k1", "v1",
				"monorail_component", "Monorail>Component",
				"monorail_project", "chromium",
				"os", "WINDOWS",
				"teamEmail", "team_email@chromium.org")
			check(ctx, cfg, tr, expected)
		})

		Convey("returns an error if artifacts are invalid", func() {
			sink, err := newSinkServer(ctx, cfg)
			So(err, ShouldBeNil)

			tr.Artifacts["art2"] = &sinkpb.Artifact{}
			_, err = sink.ReportTestResults(ctx, &sinkpb.ReportTestResultsRequest{
				TestResults: []*sinkpb.TestResult{tr}})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			closeSinkServer(ctx, sink)
		})
	})
}
