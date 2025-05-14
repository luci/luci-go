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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	"go.chromium.org/luci/resultdb/internal/schemes"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

func TestReportTestResults(t *testing.T) {
	t.Parallel()

	ctx := metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs(AuthTokenKey, authTokenValue("secret")))

	ftt.Run("ReportTestResults", t, func(t *ftt.Test) {
		// close and drain the server to enforce all the requests processed.
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		cfg := testServerConfig("", "secret")
		tr := validTestResult(t)

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

		expectedTR := proto.Clone(&pb.TestResult{
			TestIdStructured: &pb.TestIdentifier{
				ModuleName:    "module_name",
				ModuleScheme:  "scheme",
				ModuleVariant: proto.Clone(cfg.Variant).(*pb.Variant),
				CoarseName:    "coarse_name",
				FineName:      "fine_name",
				CaseName:      "component1:component2",
			},
			ResultId:            tr.ResultId,
			Expected:            tr.Expected,
			Status:              tr.Status,
			StatusV2:            tr.StatusV2,
			SummaryHtml:         tr.SummaryHtml,
			StartTime:           tr.StartTime,
			Duration:            tr.Duration,
			Tags:                tr.Tags,
			TestMetadata:        tr.TestMetadata,
			FailureReason:       tr.FailureReason,
			SkippedReason:       tr.SkippedReason,
			FrameworkExtensions: tr.FrameworkExtensions,
		}).(*pb.TestResult)

		checkResults := func() {
			sink, err := newSinkServer(ctx, cfg)
			sink.(*sinkpb.DecoratedSink).Service.(*sinkServer).resultIDBase = "foo"
			sink.(*sinkpb.DecoratedSink).Service.(*sinkServer).resultCounter = 100
			assert.Loosely(t, err, should.BeNil)
			defer closeSinkServer(ctx, sink)

			req := &sinkpb.ReportTestResultsRequest{
				TestResults: []*sinkpb.TestResult{tr},
			}
			// Clone because the RPC impl mutates the request objects.
			req = proto.Clone(req).(*sinkpb.ReportTestResultsRequest)
			_, err = sink.ReportTestResults(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			closeSinkServer(ctx, sink)
			assert.Loosely(t, sentTRReq, should.NotBeNil)
			assert.Loosely(t, sentTRReq.Requests, should.HaveLength(1))
			assert.Loosely(t, sentTRReq.Requests[0].TestResult, should.Match(expectedTR))
		}

		t.Run("with ServerConfig.BaseTags", func(t *ftt.Test) {
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

		t.Run("legacy test ID and variant", func(t *ftt.Test) {
			cfg.ModuleName = ""
			cfg.ModuleScheme = legacyScheme()

			expectedTR.TestId = "this is testID"
			expectedTR.Variant = proto.Clone(cfg.Variant).(*pb.Variant)
			expectedTR.TestIdStructured = nil

			t.Run("with shortening enabled", func(t *ftt.Test) {
				cfg.ShortenIDs = true
				assert.Loosely(t, cfg.Validate(), should.BeNil)

				t.Run("short ID", func(t *ftt.Test) {
					tr.TestId = strings.Repeat("a", 100)
					expectedTR.TestId = strings.Repeat("a", 100)
					checkResults()
				})
				t.Run("long ID, no prefix", func(t *ftt.Test) {
					tr.TestId = strings.Repeat("a", 400)
					expectedTR.TestId = strings.Repeat("a", 333) + "~abd5e54be3f59d8e"
					checkResults()
				})
				t.Run("long ID, with prefix", func(t *ftt.Test) {
					cfg.TestIDPrefix = "myprefix:"
					tr.TestId = strings.Repeat("a", 400)
					expectedTR.TestId = "myprefix:" + strings.Repeat("a", 333) + "~abd5e54be3f59d8e"
					checkResults()
				})
			})
			t.Run("without test ID prefix", func(t *ftt.Test) {
				cfg.TestIDPrefix = ""
				assert.Loosely(t, cfg.Validate(), should.BeNil)

				t.Run("with legacy-style test ID", func(t *ftt.Test) {
					tr.TestId = "HelloWorld.TestA"
					tr.TestIdStructured = nil
					expectedTR.TestId = "HelloWorld.TestA"
					expectedTR.TestIdStructured = nil
					checkResults()
				})
				t.Run("attempt to upload structured test ID via test_id field", func(t *ftt.Test) {
					// While such usage is allowed when calling ResultDB directly, it is not
					// allowed when using ResultSink as we expect the scheme to be declared
					// upfront.
					tr.TestId = ":module!myscheme:coarse:fine#method"

					sink, err := newSinkServer(ctx, cfg)
					assert.Loosely(t, err, should.BeNil)
					req := &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}}
					_, err = sink.ReportTestResults(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`test_id: module_scheme: expected test scheme "legacy" but got scheme "myscheme"`))
				})
			})
			t.Run("with Test ID Prefix", func(t *ftt.Test) {
				cfg.TestIDPrefix = "ninja://foo/bar/"
				assert.Loosely(t, cfg.Validate(), should.BeNil)

				tr.TestId = "HelloWorld.TestA"
				expectedTR.TestId = "ninja://foo/bar/HelloWorld.TestA"
				expectedTR.TestIdStructured = nil

				t.Run("with empty test ID", func(t *ftt.Test) {
					// This is allowed as the TestIDPrefix is set.
					tr.TestId = ""
					expectedTR.TestId = "ninja://foo/bar/"
					expectedTR.TestIdStructured = nil

					checkResults()
				})
				t.Run("with structured test ID uploaded in addition to legacy ID", func(t *ftt.Test) {
					tr.TestIdStructured = &sinkpb.TestIdentifier{
						CoarseName:         "coarse_name",
						FineName:           "fine_name",
						CaseNameComponents: []string{"component1", "component2"},
					}
					// Should not change result.
					checkResults()
				})
				t.Run("without structured test ID uploaded in addition to legacy ID", func(t *ftt.Test) {
					tr.TestIdStructured = nil
					checkResults()
				})

				t.Run("with ServerConfig.Variant and/or test result variant", func(t *ftt.Test) {
					v1, v2 := pbutil.Variant("bucket", "try"), pbutil.Variant("builder", "linux-rel")
					// (nil, nil)
					cfg.Variant, tr.Variant, expectedTR.Variant = nil, nil, nil
					checkResults()

					// (variant, nil)
					cfg.Variant, tr.Variant, expectedTR.Variant = v1, nil, v1
					checkResults()

					// (nil, variant)
					cfg.Variant, tr.Variant, expectedTR.Variant = nil, v1, v1
					checkResults()

					// (variant1, variant2)
					cfg.Variant, tr.Variant, expectedTR.Variant = v1, v2, pbutil.CombineVariant(v1, v2)
					checkResults()
				})
			})
		})
		t.Run("structured test ID", func(t *ftt.Test) {
			cfg.TestIDPrefix = ""
			cfg.ModuleName = "mymodule"
			cfg.ModuleScheme = testScheme("myscheme")
			assert.Loosely(t, cfg.Validate(), should.BeNil)

			expectedTR.TestId = ""
			expectedTR.TestIdStructured = &pb.TestIdentifier{
				ModuleName:    "mymodule",
				ModuleScheme:  "myscheme",
				ModuleVariant: proto.Clone(cfg.Variant).(*pb.Variant),
				CoarseName:    "coarse_name",
				FineName:      "fine_name",
				CaseName:      "component1:component2",
			}

			t.Run("with shortening enabled", func(t *ftt.Test) {
				cfg.ShortenIDs = true
				assert.Loosely(t, cfg.Validate(), should.BeNil)

				t.Run("short ID", func(t *ftt.Test) {
					checkResults()
				})
				t.Run("long ID", func(t *ftt.Test) {
					tr.TestIdStructured.CoarseName = strings.Repeat("a", 400)
					tr.TestIdStructured.FineName = strings.Repeat("b", 400)
					tr.TestIdStructured.CaseNameComponents = []string{strings.Repeat("c", 400)}
					expectedTR.TestIdStructured.CoarseName = strings.Repeat("a", 157) + "~abd5e54be3f59d8e"
					expectedTR.TestIdStructured.FineName = strings.Repeat("b", 70) + "~bc38c1c9ebc1d0b2"
					expectedTR.TestIdStructured.CaseName = strings.Repeat("c", 70) + "~cb11f55cb6f1cc7d"
					checkResults()
				})
			})
			t.Run("with empty ServerConfig.variant", func(t *ftt.Test) {
				cfg.Variant = &pb.Variant{}
				expectedTR.TestIdStructured.ModuleVariant = &pb.Variant{}
				checkResults()
			})
			t.Run("with ServerConfig.variant", func(t *ftt.Test) {
				base := []string{"bucket", "try", "builder", "linux-rel"}
				cfg.Variant = pbutil.Variant(base...)
				expectedTR.TestIdStructured.ModuleVariant = pbutil.Variant(base...)
				checkResults()
			})
			t.Run("with legacy test ID uploaded in addition to structured ID", func(t *ftt.Test) {
				tr.TestId = "HelloWorld.TestA"
				tr.Variant = pbutil.Variant("arch", "x64")

				// Legacy fields are ignored.
				checkResults()

				t.Run("with PreviousTestIdPrefix", func(t *ftt.Test) {
					previousPrefix := "ninja://oldfoo/bar/"
					cfg.PreviousTestIDPrefix = &previousPrefix

					t.Run("merges with existing test metadata", func(t *ftt.Test) {
						// Test case where metadata is merged.
						tr.TestMetadata = &pb.TestMetadata{
							Name: "MyTest",
						}
						expectedTR.TestMetadata = &pb.TestMetadata{
							Name:           "MyTest",
							PreviousTestId: "ninja://oldfoo/bar/HelloWorld.TestA",
						}
						checkResults()
					})
					t.Run("sets test metadata if it does not exist", func(t *ftt.Test) {
						// Test case where test metadata not set.
						tr.TestMetadata = nil

						expectedTR.TestMetadata = &pb.TestMetadata{
							PreviousTestId: "ninja://oldfoo/bar/HelloWorld.TestA",
						}
						checkResults()
					})
				})
			})
			t.Run("without legacy test ID uploaded in addition to structured ID", func(t *ftt.Test) {
				tr.TestId = ""
				tr.Variant = nil

				// Legacy fields are ignored.
				checkResults()

				t.Run("with PreviousTestIdPrefix", func(t *ftt.Test) {
					previousPrefix := "ninja://oldfoo/bar/"
					cfg.PreviousTestIDPrefix = &previousPrefix

					// Because no legacy test ID uploaded, no previous test ID uploaded.
					expectedTR.TestMetadata.PreviousTestId = ""
					checkResults()
				})
			})
			t.Run("with coarse name missing", func(t *ftt.Test) {
				tr.TestIdStructured.CoarseName = ""

				sink, err := newSinkServer(ctx, cfg)
				assert.Loosely(t, err, should.BeNil)
				req := &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}}
				_, err = sink.ReportTestResults(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`test_id_structured: coarse_name: required, please set a Package (scheme "myscheme")`))
			})
			t.Run("with case name components missing", func(t *ftt.Test) {
				// This often occurs if the person writing an uploader is accidentally working against
				// the ResultDB proto instead of the ResultSink proto.
				tr.TestIdStructured.CaseNameComponents = nil

				sink, err := newSinkServer(ctx, cfg)
				assert.Loosely(t, err, should.BeNil)
				req := &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}}
				_, err = sink.ReportTestResults(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`test_id_structured: case_name_components: unspecified`))
			})
		})

		t.Run("generates a random ResultID, if omitted", func(t *ftt.Test) {
			tr.ResultId = ""
			expectedTR.ResultId = "foo-00101"
			checkResults()
		})

		t.Run("duration", func(t *ftt.Test) {
			t.Run("with CoerceNegativeDuration", func(t *ftt.Test) {
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
			t.Run("without CoerceNegativeDuration", func(t *ftt.Test) {
				// duration < 0
				tr.Duration = durationpb.New(-8)
				sink, err := newSinkServer(ctx, cfg)
				assert.Loosely(t, err, should.BeNil)

				req := &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}}
				_, err = sink.ReportTestResults(ctx, req)
				assert.Loosely(t, err, should.ErrLike("duration: is < 0"))
			})
		})

		t.Run("failure reason", func(t *ftt.Test) {
			t.Run("legacy result", func(t *ftt.Test) {
				// For legacy results (using status_v1), upload of failure reasons
				// via the primary error message field was the norm.
				// The failure reason could be specified on any result, including
				// passing and skipped results. And the message could be omitted
				// for failures.
				tr.StatusV2 = pb.TestResult_STATUS_UNSPECIFIED
				tr.Status = pb.TestStatus_PASS
				tr.Expected = true

				t.Run("specified", func(t *ftt.Test) {
					tr.FailureReason = &pb.FailureReason{
						PrimaryErrorMessage: "Example failure reason.",
					}
					expectedTR.FailureReason = &pb.FailureReason{
						PrimaryErrorMessage: "Example failure reason.",
					}
					checkResults()
				})
				t.Run("primary_error_message too long", func(t *ftt.Test) {
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
					assert.Loosely(t, err, should.BeNil)

					req := &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}}
					_, err = sink.ReportTestResults(ctx, req)
					assert.Loosely(t, err, should.ErrLike(
						"failure_reason: primary_error_message: exceeds the"+
							" maximum size of 1024 bytes"))
				})
				t.Run("nil", func(t *ftt.Test) {
					tr.FailureReason = nil
					expectedTR.FailureReason = nil
					checkResults()
				})
			})

			tr.StatusV2 = pb.TestResult_FAILED
			tr.Status = pb.TestStatus_STATUS_UNSPECIFIED
			tr.Expected = false

			expectedTR.StatusV2 = pb.TestResult_FAILED
			expectedTR.Status = pb.TestStatus_STATUS_UNSPECIFIED
			expectedTR.Expected = false

			tr.FailureReason = &pb.FailureReason{
				Kind: pb.FailureReason_ORDINARY,
				Errors: []*pb.FailureReason_Error{
					{Message: "Example failure reason."},
					{Message: "Example failure reason2."},
				},
				TruncatedErrorsCount: 0,
			}
			expectedTR.FailureReason = &pb.FailureReason{
				Kind: pb.FailureReason_ORDINARY,
				Errors: []*pb.FailureReason_Error{
					{Message: "Example failure reason."},
					{Message: "Example failure reason2."},
				},
				TruncatedErrorsCount: 0,
			}

			t.Run("specified", func(t *ftt.Test) {
				checkResults()
			})
			t.Run("kind unspecified", func(t *ftt.Test) {
				tr.FailureReason.Kind = pb.FailureReason_KIND_UNSPECIFIED

				sink, err := newSinkServer(ctx, cfg)
				assert.Loosely(t, err, should.BeNil)

				req := &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}}
				_, err = sink.ReportTestResults(ctx, req)
				assert.Loosely(t, err, should.ErrLike("failure_reason: kind: unspecified"))
			})
			t.Run("unspecified for failures", func(t *ftt.Test) {
				tr.FailureReason = nil
				expectedTR.FailureReason = nil

				sink, err := newSinkServer(ctx, cfg)
				assert.Loosely(t, err, should.BeNil)

				req := &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}}
				_, err = sink.ReportTestResults(ctx, req)
				assert.Loosely(t, err, should.ErrLike("failure_reason: must be set when status_v2 is FAILED"))
			})

			t.Run("error_messages too long", func(t *ftt.Test) {
				var b strings.Builder
				// Make a string that exceeds the 1024-byte length limit
				// (when encoded as UTF-8).
				for i := 0; i < 1025; i++ {
					b.WriteRune('.')
				}
				tr.FailureReason = &pb.FailureReason{
					Kind: pb.FailureReason_CRASH,
					Errors: []*pb.FailureReason_Error{
						{Message: "Example failure reason."},
						{Message: b.String()},
					},
					TruncatedErrorsCount: 0,
				}

				sink, err := newSinkServer(ctx, cfg)
				assert.Loosely(t, err, should.BeNil)

				req := &sinkpb.ReportTestResultsRequest{
					TestResults: []*sinkpb.TestResult{tr},
				}
				_, err = sink.ReportTestResults(ctx, req)
				assert.Loosely(t, err, should.ErrLike(
					fmt.Sprintf("errors[1]: message: exceeds the maximum "+
						"size of 1024 bytes")))
			})
		})

		t.Run("properties", func(t *ftt.Test) {
			t.Run("specified", func(t *ftt.Test) {
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

			t.Run("nil", func(t *ftt.Test) {
				tr.Properties = nil
				expectedTR.Properties = nil
				checkResults()
			})

			t.Run("properties too large", func(t *ftt.Test) {
				tr.Properties = &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key1": structpb.NewStringValue(strings.Repeat("1", pbutil.MaxSizeTestResultProperties)),
					},
				}

				sink, err := newSinkServer(ctx, cfg)
				assert.Loosely(t, err, should.BeNil)

				req := &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}}
				_, err = sink.ReportTestResults(ctx, req)
				assert.Loosely(t, err, should.ErrLike(`exceeds the maximum size of`))
				assert.Loosely(t, err, should.ErrLike(`bytes`))
			})
		})

		t.Run("with ServerConfig.TestLocationBase", func(t *ftt.Test) {
			cfg.TestLocationBase = "//base/"
			tr.TestMetadata.Location.FileName = "artifact_dir/a_test.cc"
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
			"team_email", "team_email@chromium.org",
			"os", "WINDOWS",
		)
		rootComponent := &pb.BugComponent{
			System: &pb.BugComponent_IssueTracker{
				IssueTracker: &pb.IssueTrackerComponent{
					ComponentId: 111,
				},
			},
		}

		t.Run("with ServerConfig.LocationTags", func(t *ftt.Test) {
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
			// Let the bug component be derived from the location.
			tr.TestMetadata.BugComponent = nil

			expectedTR.Tags = append(expectedTR.Tags, pbutil.StringPairs(
				"feature", "feature2",
				"feature", "feature3",
				"monorail_component", "Monorail>Component>Sub",
				"team_email", "team_email@chromium.org",
				"os", "WINDOWS",
			)...)
			expectedTR.TestMetadata.BugComponent = subComponent
			pbutil.SortStringPairs(expectedTR.Tags)
			checkResults()
		})

		t.Run("with ServerConfig.LocationTags file based", func(t *ftt.Test) {
			overriddenTags := pbutil.StringPairs(
				"feature_x", "featureY",
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
			// Let the bug component be derived from the location.
			tr.TestMetadata.BugComponent = nil

			expectedTR.Tags = append(expectedTR.Tags, pbutil.StringPairs(
				"feature", "feature2",
				"feature", "feature3",
				"feature_x", "featureY",
				"monorail_component", "Monorail>File>Component",
				"team_email", "team_email@chromium.org",
				"os", "WINDOWS",
			)...)
			expectedTR.TestMetadata.BugComponent = overriddenComponent
			pbutil.SortStringPairs(expectedTR.Tags)

			checkResults()
		})

		t.Run("ReportTestResults", func(t *ftt.Test) {
			sink, err := newSinkServer(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)
			defer closeSinkServer(ctx, sink)

			report := func(trs ...*sinkpb.TestResult) error {
				_, err := sink.ReportTestResults(ctx, &sinkpb.ReportTestResultsRequest{TestResults: trs})
				return err
			}

			t.Run("returns an error if the artifact req is invalid", func(t *ftt.Test) {
				tr.Artifacts["art2"] = &sinkpb.Artifact{}
				err := report(tr)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("one of file_path or contents or gcs_uri must be provided"))
			})

			t.Run("with an inaccesible artifact file", func(t *ftt.Test) {
				tr.Artifacts["art2"] = &sinkpb.Artifact{
					Body: &sinkpb.Artifact_FilePath{FilePath: "not_exist"}}

				t.Run("drops the artifact", func(t *ftt.Test) {
					assert.Loosely(t, report(tr), grpccode.ShouldBe(codes.OK))

					// make sure that no TestResults were dropped, and the valid artifact, "art1",
					// was not dropped, either.
					closeSinkServer(ctx, sink)
					assert.Loosely(t, sentTRReq, should.NotBeNil)
					assert.Loosely(t, sentTRReq.Requests, should.HaveLength(1))
					assert.Loosely(t, sentTRReq.Requests[0].TestResult, should.Match(expectedTR))

					assert.Loosely(t, sentArtReq, should.NotBeNil)
					assert.Loosely(t, sentArtReq.Requests, should.HaveLength(1))
					assert.Loosely(t, sentArtReq.Requests[0].Parent, should.HaveSuffix("/tests/:module_name%21scheme:coarse_name:fine_name%23component1:component2/results/result_id1"))
					assert.Loosely(t, sentArtReq.Requests[0].Artifact, should.Match(&pb.Artifact{
						ArtifactId:  "art1",
						ContentType: "text/plain",
						Contents:    []byte("a sample artifact"),
						SizeBytes:   int64(len("a sample artifact")),
						TestStatus:  pb.TestStatus_PASS,
					}))
				})
			})
		})

		t.Run("report exoneration", func(t *ftt.Test) {
			t.Run("legacy test ID", func(t *ftt.Test) {
				cfg.ModuleName = ""
				cfg.ModuleScheme = legacyScheme()
				cfg.Variant = pbutil.Variant("bucket", "try", "builder", "linux-rel")
				cfg.ExonerateUnexpectedPass = true

				sink, err := newSinkServer(ctx, cfg)
				assert.Loosely(t, err, should.BeNil)
				defer closeSinkServer(ctx, sink)

				tr.Expected = false

				_, err = sink.ReportTestResults(ctx, &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
				closeSinkServer(ctx, sink)
				assert.Loosely(t, sentExoReq, should.NotBeNil)
				assert.Loosely(t, sentExoReq.Requests, should.HaveLength(1))
				assert.Loosely(t, sentExoReq.Requests[0].TestExoneration, should.Match(&pb.TestExoneration{
					TestId:          tr.TestId,
					Variant:         pbutil.Variant("bucket", "try", "builder", "linux-rel"),
					ExplanationHtml: "Unexpected passes are exonerated",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
				}))
			})
			t.Run("structured test ID", func(t *ftt.Test) {
				cfg.ModuleName = "modulename"
				cfg.ModuleScheme = testScheme("scheme")
				cfg.Variant = pbutil.Variant("bucket", "try", "builder", "linux-rel")
				cfg.ExonerateUnexpectedPass = true

				sink, err := newSinkServer(ctx, cfg)
				assert.Loosely(t, err, should.BeNil)
				defer closeSinkServer(ctx, sink)

				t.Run("exonerate unexpected pass", func(t *ftt.Test) {
					tr.Expected = false

					_, err = sink.ReportTestResults(ctx, &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}})
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					closeSinkServer(ctx, sink)
					assert.Loosely(t, sentExoReq, should.NotBeNil)
					assert.Loosely(t, sentExoReq.Requests, should.HaveLength(1))
					assert.Loosely(t, sentExoReq.Requests[0].TestExoneration, should.Match(&pb.TestExoneration{
						TestIdStructured: &pb.TestIdentifier{
							ModuleName:    "modulename",
							ModuleScheme:  "scheme",
							ModuleVariant: pbutil.Variant("bucket", "try", "builder", "linux-rel"),
							CoarseName:    "coarse_name",
							FineName:      "fine_name",
							CaseName:      "component1:component2",
						},
						ExplanationHtml: "Unexpected passes are exonerated",
						Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
					}))
				})

				t.Run("not exonerate unexpected failure", func(t *ftt.Test) {
					tr.Expected = false
					tr.Status = pb.TestStatus_FAIL

					_, err = sink.ReportTestResults(ctx, &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}})
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					closeSinkServer(ctx, sink)
					assert.Loosely(t, sentExoReq, should.BeNil)
				})

				t.Run("not exonerate expected pass", func(t *ftt.Test) {
					_, err = sink.ReportTestResults(ctx, &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}})
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					closeSinkServer(ctx, sink)
					assert.Loosely(t, sentExoReq, should.BeNil)
				})

				t.Run("not exonerate expected failure", func(t *ftt.Test) {
					tr.Status = pb.TestStatus_FAIL

					_, err = sink.ReportTestResults(ctx, &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}})
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					closeSinkServer(ctx, sink)
					assert.Loosely(t, sentExoReq, should.BeNil)
				})
			})
		})
	})
}

func TestReportInvocationLevelArtifacts(t *testing.T) {
	t.Parallel()

	ftt.Run("ReportInvocationLevelArtifacts", t, func(t *ftt.Test) {
		ctx := metadata.NewIncomingContext(
			context.Background(),
			metadata.Pairs(AuthTokenKey, authTokenValue("secret")))
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		cfg := testServerConfig("", "secret")
		sink, err := newSinkServer(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)
		defer closeSinkServer(ctx, sink)

		art1 := &sinkpb.Artifact{Body: &sinkpb.Artifact_Contents{Contents: []byte("123")}}
		art2 := &sinkpb.Artifact{Body: &sinkpb.Artifact_GcsUri{GcsUri: "gs://bucket/foo"}}

		req := &sinkpb.ReportInvocationLevelArtifactsRequest{
			Artifacts: map[string]*sinkpb.Artifact{"art1": art1, "art2": art2},
		}
		_, err = sink.ReportInvocationLevelArtifacts(ctx, req)
		assert.Loosely(t, err, should.BeNil)

		// Duplicated artifact will be rejected.
		_, err = sink.ReportInvocationLevelArtifacts(ctx, req)
		assert.Loosely(t, err, should.ErrLike(` has already been uploaded`))
	})
}

func TestUpdateInvocation(t *testing.T) {
	t.Parallel()

	ftt.Run("UpdateInvocation", t, func(t *ftt.Test) {
		ctx := metadata.NewIncomingContext(
			context.Background(),
			metadata.Pairs(AuthTokenKey, authTokenValue("secret")))
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		cfg := testServerConfig("", "secret")
		sink, err := newSinkServer(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)
		defer closeSinkServer(ctx, sink)

		sinkInv := &sinkpb.Invocation{
			ExtendedProperties: map[string]*structpb.Struct{
				"abc": {
					Fields: map[string]*structpb.Value{
						"@type":     structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
						"child_key": structpb.NewStringValue("child_value"),
					},
				},
			},
		}

		t.Run("invalid update mask", func(t *ftt.Test) {
			req := &sinkpb.UpdateInvocationRequest{
				Invocation: sinkInv,
				UpdateMask: &fieldmaskpb.FieldMask{
					Paths: []string{"deadline"},
				},
			}
			_, err := sink.UpdateInvocation(ctx, req)
			assert.Loosely(t, err, should.ErrLike("update_mask"))
			assert.Loosely(t, err, should.ErrLike("does not exist in message Invocation"))
		})

		t.Run("valid update mask", func(t *ftt.Test) {
			req := &sinkpb.UpdateInvocationRequest{
				Invocation: sinkInv,
				UpdateMask: &fieldmaskpb.FieldMask{
					Paths: []string{"extended_properties.abc"},
				},
			}
			_, err := sink.UpdateInvocation(ctx, req)
			assert.Loosely(t, err, should.BeNil)
		})
	})
}

// legacyScheme returns the 'legacy' Scheme for testing.
func legacyScheme() *schemes.Scheme {
	legacyScheme, err := schemes.FromProto(&pb.Scheme{
		Id:                pbutil.LegacySchemeID,
		HumanReadableName: "Legacy test results",
		Case: &pb.Scheme_Level{
			HumanReadableName: "Test Identifier",
		},
	})
	if err != nil {
		panic(err)
	}
	return legacyScheme
}

// testScheme returns the a Scheme for testing.
func testScheme(id string) *schemes.Scheme {
	scheme, err := schemes.FromProto(&pb.Scheme{
		Id:                id,
		HumanReadableName: "My scheme",
		Coarse: &pb.Scheme_Level{
			HumanReadableName: "Package",
		},
		Fine: &pb.Scheme_Level{
			HumanReadableName: "Class",
		},
		Case: &pb.Scheme_Level{
			HumanReadableName: "Method",
		},
	})
	if err != nil {
		panic(err)
	}
	return scheme
}
