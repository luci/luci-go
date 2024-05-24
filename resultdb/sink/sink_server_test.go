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
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

func TestReportTestResults(t *testing.T) {
	t.Parallel()

	ctx := metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs(AuthTokenKey, authTokenValue("secret")))

	Convey("ReportTestResults", t, func() {
		// close and drain the server to enforce all the requests processed.
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		cfg := testServerConfig("", "secret")
		tr, cleanup := validTestResult()
		defer cleanup()

		var sentTRReq *pb.BatchCreateTestResultsRequest
		cfg.Recorder.(*mockRecorder).batchCreateTestResults = func(c context.Context, in *pb.BatchCreateTestResultsRequest) (*pb.BatchCreateTestResultsResponse, error) {
			sentTRReq = in
			return nil, nil
		}
		var sentArtReq *pb.BatchCreateArtifactsRequest
		cfg.Recorder.(*mockRecorder).batchCreateArtifacts = func(ctx context.Context, in *pb.BatchCreateArtifactsRequest) (*pb.BatchCreateArtifactsResponse, error) {
			sentArtReq = in
			return nil, nil
		}
		var sentExoReq *pb.BatchCreateTestExonerationsRequest
		cfg.Recorder.(*mockRecorder).batchCreateTestExonerations = func(ctx context.Context, in *pb.BatchCreateTestExonerationsRequest) (*pb.BatchCreateTestExonerationsResponse, error) {
			sentExoReq = in
			return nil, nil
		}

		expectedTR := &pb.TestResult{
			TestId:        tr.TestId,
			ResultId:      tr.ResultId,
			Expected:      tr.Expected,
			Status:        tr.Status,
			SummaryHtml:   tr.SummaryHtml,
			StartTime:     tr.StartTime,
			Duration:      tr.Duration,
			Tags:          tr.Tags,
			Variant:       tr.Variant,
			TestMetadata:  tr.TestMetadata,
			FailureReason: tr.FailureReason,
		}

		checkResults := func() {
			sink, err := newSinkServer(ctx, cfg)
			sink.(*sinkpb.DecoratedSink).Service.(*sinkServer).resultIDBase = "foo"
			sink.(*sinkpb.DecoratedSink).Service.(*sinkServer).resultCounter = 100
			So(err, ShouldBeNil)
			defer closeSinkServer(ctx, sink)

			req := &sinkpb.ReportTestResultsRequest{
				TestResults: []*sinkpb.TestResult{tr},
			}
			// Clone because the RPC impl mutates the request objects.
			req = proto.Clone(req).(*sinkpb.ReportTestResultsRequest)
			_, err = sink.ReportTestResults(ctx, req)
			So(err, ShouldBeNil)

			closeSinkServer(ctx, sink)
			So(sentTRReq, ShouldNotBeNil)
			So(sentTRReq.Requests, ShouldHaveLength, 1)
			So(sentTRReq.Requests[0].TestResult, ShouldResembleProto, expectedTR)
		}

		Convey("works", func() {
			Convey("with ServerConfig.TestIDPrefix", func() {
				cfg.TestIDPrefix = "ninja://foo/bar/"
				tr.TestId = "HelloWorld.TestA"
				expectedTR.TestId = "ninja://foo/bar/HelloWorld.TestA"
				checkResults()
			})

			Convey("with ServerConfig.BaseVariant", func() {
				base := []string{"bucket", "try", "builder", "linux-rel"}
				cfg.BaseVariant = pbutil.Variant(base...)
				expectedTR.Variant = pbutil.Variant(base...)
				checkResults()
			})

			Convey("with ServerConfig.BaseTags", func() {
				t1, t2 := pbutil.StringPairs("t1", "v1"), pbutil.StringPairs("t2", "v2")
				// (nil, nil)
				cfg.BaseTags, tr.Tags, expectedTR.Tags = nil, nil, nil
				checkResults()

				// (tag, nil)
				cfg.BaseTags, tr.Tags, expectedTR.Tags = t1, nil, t1
				checkResults()

				// (nil, tag)
				cfg.BaseTags, tr.Tags, expectedTR.Tags = nil, t1, t1
				checkResults()

				// (tag1, tag2)
				cfg.BaseTags, tr.Tags, expectedTR.Tags = t1, t2, append(t1, t2...)
				checkResults()
			})

			Convey("with ServerConfig.BaseVariant and test result variant", func() {
				v1, v2 := pbutil.Variant("bucket", "try"), pbutil.Variant("builder", "linux-rel")
				// (nil, nil)
				cfg.BaseVariant, tr.Variant, expectedTR.Variant = nil, nil, nil
				checkResults()

				// (variant, nil)
				cfg.BaseVariant, tr.Variant, expectedTR.Variant = v1, nil, v1
				checkResults()

				// (nil, variant)
				cfg.BaseVariant, tr.Variant, expectedTR.Variant = nil, v1, v1
				checkResults()

				// (variant1, variant2)
				cfg.BaseVariant, tr.Variant, expectedTR.Variant = v1, v2, pbutil.CombineVariant(v1, v2)
				checkResults()
			})
		})

		Convey("generates a random ResultID, if omitted", func() {
			tr.ResultId = ""
			expectedTR.ResultId = "foo-00101"
			checkResults()
		})

		Convey("duration", func() {
			Convey("with CoerceNegativeDuration", func() {
				cfg.CoerceNegativeDuration = true

				// duration == nil
				tr.Duration, expectedTR.Duration = nil, nil
				checkResults()

				// duration == 0
				tr.Duration, expectedTR.Duration = durationpb.New(0), durationpb.New(0)
				checkResults()

				// duration > 0
				tr.Duration, expectedTR.Duration = durationpb.New(8), durationpb.New(8)
				checkResults()

				// duration < 0
				tr.Duration = durationpb.New(-8)
				expectedTR.Duration = durationpb.New(0)
				checkResults()
			})
			Convey("without CoerceNegativeDuration", func() {
				// duration < 0
				tr.Duration = durationpb.New(-8)
				sink, err := newSinkServer(ctx, cfg)
				So(err, ShouldBeNil)

				req := &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}}
				_, err = sink.ReportTestResults(ctx, req)
				So(err, ShouldErrLike, "duration: is < 0")
			})
		})

		Convey("failure reason", func() {
			Convey("specified", func() {
				tr.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: "Example failure reason.",
					Errors: []*pb.FailureReason_Error{
						{Message: "Example failure reason."},
						{Message: "Example failure reason2."},
					},
					TruncatedErrorsCount: 0,
				}
				expectedTR.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: "Example failure reason.",
					Errors: []*pb.FailureReason_Error{
						{Message: "Example failure reason."},
						{Message: "Example failure reason2."},
					},
					TruncatedErrorsCount: 0,
				}
				checkResults()
			})

			Convey("nil", func() {
				tr.FailureReason = nil
				expectedTR.FailureReason = nil
				checkResults()
			})

			Convey("primary_error_message too long", func() {
				var b strings.Builder
				// Make a string that exceeds the 1024-byte length limit
				// (when encoded as UTF-8).
				for i := 0; i < 1025; i++ {
					b.WriteRune('.')
				}
				tr.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: b.String(),
				}

				sink, err := newSinkServer(ctx, cfg)
				So(err, ShouldBeNil)

				req := &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}}
				_, err = sink.ReportTestResults(ctx, req)
				So(err, ShouldErrLike,
					"failure_reason: primary_error_message: exceeds the"+
						" maximum size of 1024 bytes")
			})

			Convey("error_messages too long", func() {
				var b strings.Builder
				// Make a string that exceeds the 1024-byte length limit
				// (when encoded as UTF-8).
				for i := 0; i < 1025; i++ {
					b.WriteRune('.')
				}
				tr.FailureReason = &pb.FailureReason{
					PrimaryErrorMessage: "Example failure reason.",
					Errors: []*pb.FailureReason_Error{
						{Message: "Example failure reason."},
						{Message: b.String()},
					},
					TruncatedErrorsCount: 0,
				}

				sink, err := newSinkServer(ctx, cfg)
				So(err, ShouldBeNil)

				req := &sinkpb.ReportTestResultsRequest{
					TestResults: []*sinkpb.TestResult{tr},
				}
				_, err = sink.ReportTestResults(ctx, req)
				So(err, ShouldErrLike,
					fmt.Sprintf("errors[1]: message: exceeds the maximum "+
						"size of 1024 bytes"))
			})
		})

		Convey("properties", func() {
			Convey("specified", func() {
				tr.Properties = &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key_1": structpb.NewStringValue("value_1"),
						"key_2": structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{
								"child_key": structpb.NewNumberValue(1),
							},
						}),
					},
				}
				expectedTR.Properties = &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key_1": structpb.NewStringValue("value_1"),
						"key_2": structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{
								"child_key": structpb.NewNumberValue(1),
							},
						}),
					},
				}
				checkResults()
			})

			Convey("nil", func() {
				tr.Properties = nil
				expectedTR.Properties = nil
				checkResults()
			})

			Convey("properties too large", func() {
				tr.Properties = &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key1": structpb.NewStringValue(strings.Repeat("1", pbutil.MaxSizeTestResultProperties)),
					},
				}

				sink, err := newSinkServer(ctx, cfg)
				So(err, ShouldBeNil)

				req := &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}}
				_, err = sink.ReportTestResults(ctx, req)
				So(err, ShouldErrLike, `properties: exceeds the maximum size of`, `bytes`)
			})
		})

		Convey("with ServerConfig.TestLocationBase", func() {
			cfg.TestLocationBase = "//base/"
			tr.TestMetadata.Location.FileName = "artifact_dir/a_test.cc"
			expectedTR.TestMetadata = proto.Clone(expectedTR.TestMetadata).(*pb.TestMetadata)
			expectedTR.TestMetadata.Location.FileName = "//base/artifact_dir/a_test.cc"
			checkResults()
		})

		subTags := pbutil.StringPairs(
			"feature", "feature2",
			"feature", "feature3",
			"monorail_component", "Monorail>Component>Sub",
		)
		subComponent := &pb.BugComponent{
			System: &pb.BugComponent_IssueTracker{
				IssueTracker: &pb.IssueTrackerComponent{
					ComponentId: 222,
				},
			},
		}

		rootTags := pbutil.StringPairs(
			"feature", "feature1",
			"monorail_component", "Monorail>Component",
			"teamEmail", "team_email@chromium.org",
			"os", "WINDOWS",
		)
		rootComponent := &pb.BugComponent{
			System: &pb.BugComponent_IssueTracker{
				IssueTracker: &pb.IssueTrackerComponent{
					ComponentId: 111,
				},
			},
		}

		Convey("with ServerConfig.LocationTags", func() {
			cfg.LocationTags = &sinkpb.LocationTags{
				Repos: map[string]*sinkpb.LocationTags_Repo{
					"https://chromium.googlesource.com/chromium/src": {
						Dirs: map[string]*sinkpb.LocationTags_Dir{
							".": {
								Tags:         rootTags,
								BugComponent: rootComponent,
							},
							"artifact_dir": {
								Tags:         subTags,
								BugComponent: subComponent,
							},
						},
					},
				},
			}
			expectedTR.Tags = append(expectedTR.Tags, pbutil.StringPairs(
				"feature", "feature2",
				"feature", "feature3",
				"monorail_component", "Monorail>Component>Sub",
				"teamEmail", "team_email@chromium.org",
				"os", "WINDOWS",
			)...)
			expectedTR.TestMetadata.BugComponent = subComponent
			pbutil.SortStringPairs(expectedTR.Tags)
			checkResults()
		})

		Convey("with ServerConfig.LocationTags file based", func() {
			overriddenTags := pbutil.StringPairs(
				"featureX", "featureY",
				"monorail_component", "Monorail>File>Component",
			)
			overriddenComponent := &pb.BugComponent{
				System: &pb.BugComponent_IssueTracker{
					IssueTracker: &pb.IssueTrackerComponent{
						ComponentId: 333,
					},
				},
			}

			cfg.LocationTags = &sinkpb.LocationTags{
				Repos: map[string]*sinkpb.LocationTags_Repo{
					"https://chromium.googlesource.com/chromium/src": {
						Files: map[string]*sinkpb.LocationTags_File{
							"artifact_dir/a_test.cc": {
								Tags:         overriddenTags,
								BugComponent: overriddenComponent,
							},
						},
						Dirs: map[string]*sinkpb.LocationTags_Dir{
							".": {
								Tags:         rootTags,
								BugComponent: rootComponent,
							},
							"artifact_dir": {
								Tags:         subTags,
								BugComponent: subComponent,
							},
						},
					},
				},
			}

			expectedTR.Tags = append(expectedTR.Tags, pbutil.StringPairs(
				"feature", "feature2",
				"feature", "feature3",
				"featureX", "featureY",
				"monorail_component", "Monorail>File>Component",
				"teamEmail", "team_email@chromium.org",
				"os", "WINDOWS",
			)...)
			expectedTR.TestMetadata.BugComponent = overriddenComponent
			pbutil.SortStringPairs(expectedTR.Tags)

			checkResults()
		})

		Convey("ReportTestResults", func() {
			sink, err := newSinkServer(ctx, cfg)
			So(err, ShouldBeNil)
			defer closeSinkServer(ctx, sink)

			report := func(trs ...*sinkpb.TestResult) error {
				_, err := sink.ReportTestResults(ctx, &sinkpb.ReportTestResultsRequest{TestResults: trs})
				return err
			}

			Convey("returns an error if the artifact req is invalid", func() {
				tr.Artifacts["art2"] = &sinkpb.Artifact{}
				So(report(tr), ShouldHaveRPCCode, codes.InvalidArgument,
					"one of file_path or contents or gcs_uri must be provided")
			})

			Convey("with an inaccesible artifact file", func() {
				tr.Artifacts["art2"] = &sinkpb.Artifact{
					Body: &sinkpb.Artifact_FilePath{FilePath: "not_exist"}}

				Convey("drops the artifact", func() {
					So(report(tr), ShouldBeRPCOK)

					// make sure that no TestResults were dropped, and the valid artifact, "art1",
					// was not dropped, either.
					closeSinkServer(ctx, sink)
					So(sentTRReq, ShouldNotBeNil)
					So(sentTRReq.Requests, ShouldHaveLength, 1)
					So(sentTRReq.Requests[0].TestResult, ShouldResembleProto, expectedTR)

					So(sentArtReq, ShouldNotBeNil)
					So(sentArtReq.Requests, ShouldHaveLength, 1)
					So(sentArtReq.Requests[0].Artifact, ShouldResembleProto, &pb.Artifact{
						ArtifactId:  "art1",
						ContentType: "text/plain",
						Contents:    []byte("a sample artifact"),
						SizeBytes:   int64(len("a sample artifact")),
						TestStatus:  pb.TestStatus_PASS,
					})
				})
			})
		})

		Convey("report exoneration", func() {
			cfg.ExonerateUnexpectedPass = true
			sink, err := newSinkServer(ctx, cfg)
			So(err, ShouldBeNil)
			defer closeSinkServer(ctx, sink)

			Convey("exonerate unexpected pass", func() {
				tr.Expected = false

				_, err = sink.ReportTestResults(ctx, &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}})
				So(err, ShouldBeRPCOK)
				closeSinkServer(ctx, sink)
				So(sentExoReq, ShouldNotBeNil)
				So(sentExoReq.Requests, ShouldHaveLength, 1)
				So(sentExoReq.Requests[0].TestExoneration, ShouldResembleProto, &pb.TestExoneration{
					TestId:          tr.TestId,
					ExplanationHtml: "Unexpected passes are exonerated",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
				})
			})

			Convey("not exonerate unexpected failure", func() {
				tr.Expected = false
				tr.Status = pb.TestStatus_FAIL

				_, err = sink.ReportTestResults(ctx, &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}})
				So(err, ShouldBeRPCOK)
				closeSinkServer(ctx, sink)
				So(sentExoReq, ShouldBeNil)
			})

			Convey("not exonerate expected pass", func() {
				_, err = sink.ReportTestResults(ctx, &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}})
				So(err, ShouldBeRPCOK)
				closeSinkServer(ctx, sink)
				So(sentExoReq, ShouldBeNil)
			})

			Convey("not exonerate expected failure", func() {
				tr.Status = pb.TestStatus_FAIL

				_, err = sink.ReportTestResults(ctx, &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}})
				So(err, ShouldBeRPCOK)
				closeSinkServer(ctx, sink)
				So(sentExoReq, ShouldBeNil)
			})
		})
	})
}

func TestReportInvocationLevelArtifacts(t *testing.T) {
	t.Parallel()

	Convey("ReportInvocationLevelArtifacts", t, func() {
		ctx := metadata.NewIncomingContext(
			context.Background(),
			metadata.Pairs(AuthTokenKey, authTokenValue("secret")))
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		cfg := testServerConfig("", "secret")
		sink, err := newSinkServer(ctx, cfg)
		So(err, ShouldBeNil)
		defer closeSinkServer(ctx, sink)

		art1 := &sinkpb.Artifact{Body: &sinkpb.Artifact_Contents{Contents: []byte("123")}}
		art2 := &sinkpb.Artifact{Body: &sinkpb.Artifact_GcsUri{GcsUri: "gs://bucket/foo"}}

		req := &sinkpb.ReportInvocationLevelArtifactsRequest{
			Artifacts: map[string]*sinkpb.Artifact{"art1": art1, "art2": art2},
		}
		_, err = sink.ReportInvocationLevelArtifacts(ctx, req)
		So(err, ShouldBeNil)

		// Duplicated artifact will be rejected.
		_, err = sink.ReportInvocationLevelArtifacts(ctx, req)
		So(err, ShouldErrLike, ` has already been uploaded`)
	})
}

func TestUpdateInvocation(t *testing.T) {
	t.Parallel()

	Convey("UpdateInvocation", t, func() {
		ctx := metadata.NewIncomingContext(
			context.Background(),
			metadata.Pairs(AuthTokenKey, authTokenValue("secret")))
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		cfg := testServerConfig("", "secret")
		sink, err := newSinkServer(ctx, cfg)
		So(err, ShouldBeNil)
		defer closeSinkServer(ctx, sink)

		sinkInv := &sinkpb.Invocation{
			ExtendedProperties: map[string]*structpb.Struct{
				"abc": &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"@type":     structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
						"child_key": structpb.NewStringValue("child_value"),
					},
				},
			},
		}

		Convey("invalid update mask", func() {
			req := &sinkpb.UpdateInvocationRequest{
				Invocation: sinkInv,
				UpdateMask: &fieldmaskpb.FieldMask{
					Paths: []string{"deadline"},
				},
			}
			_, err := sink.UpdateInvocation(ctx, req)
			So(err, ShouldErrLike, "update_mask", "does not exist in message Invocation")
		})

		Convey("valid update mask", func() {
			req := &sinkpb.UpdateInvocationRequest{
				Invocation: sinkInv,
				UpdateMask: &fieldmaskpb.FieldMask{
					Paths: []string{"extended_properties.abc"},
				},
			}
			_, err := sink.UpdateInvocation(ctx, req)
			So(err, ShouldBeNil)
		})
	})
}
