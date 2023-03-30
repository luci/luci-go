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

package recorder

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestValidateUpdateInvocationRequest(t *testing.T) {
	t.Parallel()
	now := testclock.TestRecentTimeUTC
	Convey(`TestValidateUpdateInvocationRequest`, t, func() {
		request := &pb.UpdateInvocationRequest{
			Invocation: &pb.Invocation{
				Name: "invocations/inv",
			},
			UpdateMask: &field_mask.FieldMask{Paths: []string{}},
		}

		Convey(`empty`, func() {
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{}, now)
			So(err, ShouldErrLike, `invocation: name: unspecified`)
		})

		Convey(`invalid id`, func() {
			request.Invocation.Name = "1"
			err := validateUpdateInvocationRequest(request, now)
			So(err, ShouldErrLike, `invocation: name: does not match`)
		})

		Convey(`empty update mask`, func() {
			err := validateUpdateInvocationRequest(request, now)
			So(err, ShouldErrLike, `update_mask: paths is empty`)
		})

		Convey(`unsupported update mask`, func() {
			request.UpdateMask.Paths = []string{"name"}
			err := validateUpdateInvocationRequest(request, now)
			So(err, ShouldErrLike, `update_mask: unsupported path "name"`)
		})

		Convey(`deadline`, func() {
			request.UpdateMask.Paths = []string{"deadline"}

			Convey(`invalid`, func() {
				deadline := pbutil.MustTimestampProto(now.Add(-time.Hour))
				request.Invocation.Deadline = deadline
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldErrLike, `invocation: deadline: must be at least 10 seconds in the future`)
			})

			Convey(`valid`, func() {
				deadline := pbutil.MustTimestampProto(now.Add(time.Hour))
				request.Invocation.Deadline = deadline
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})
		})
		Convey(`bigquery exports`, func() {
			request.UpdateMask = &field_mask.FieldMask{Paths: []string{"bigquery_exports"}}

			Convey(`invalid`, func() {
				request.Invocation.BigqueryExports = []*pb.BigQueryExport{{
					Project: "project",
					Dataset: "dataset",
					Table:   "table",
					// No ResultType.
				}}
				request.UpdateMask.Paths = []string{"bigquery_exports"}
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldErrLike, `invocation: bigquery_exports[0]: result_type: unspecified`)
			})

			Convey(`valid`, func() {
				request.Invocation.BigqueryExports = []*pb.BigQueryExport{{
					Project: "project",
					Dataset: "dataset",
					Table:   "table",
					ResultType: &pb.BigQueryExport_TestResults_{
						TestResults: &pb.BigQueryExport_TestResults{},
					},
				}}
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})

			Convey(`empty`, func() {
				request.Invocation.BigqueryExports = []*pb.BigQueryExport{}
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})
		})
		Convey(`properties`, func() {
			request.UpdateMask.Paths = []string{"properties"}

			Convey(`invalid`, func() {
				request.Invocation.Properties = &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key1": structpb.NewStringValue(strings.Repeat("1", pbutil.MaxSizeProperties)),
					},
				}
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldErrLike, `invocation: properties: exceeds the maximum size of`, `bytes`)
			})
			Convey(`valid`, func() {
				request.Invocation.Properties = &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key_1": structpb.NewStringValue("value_1"),
						"key_2": structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{
								"child_key": structpb.NewNumberValue(1),
							},
						}),
					},
				}
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})
		})
		Convey(`source spec`, func() {
			request.UpdateMask.Paths = []string{"source_spec"}

			Convey(`valid`, func() {
				request.Invocation.SourceSpec = &pb.SourceSpec{
					Sources: &pb.Sources{
						GitilesCommit: &pb.GitilesCommit{
							Host:       "chromium.googlesource.com",
							Project:    "infra/infra",
							Ref:        "refs/heads/main",
							CommitHash: "1234567890abcdefabcd1234567890abcdefabcd",
							Position:   567,
						},
						Changelists: []*pb.GerritChange{
							{
								Host:     "chromium-review.googlesource.com",
								Project:  "infra/luci-go",
								Change:   12345,
								Patchset: 321,
							},
						},
						IsDirty: true,
					},
				}
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})

			Convey(`invalid source spec`, func() {
				request.Invocation.SourceSpec = &pb.SourceSpec{
					Sources: &pb.Sources{
						GitilesCommit: &pb.GitilesCommit{},
					},
				}
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldErrLike, `invocation: source_spec: sources: gitiles_commit: host: unspecified`)
			})
		})
	})
}

func TestUpdateInvocation(t *testing.T) {
	Convey(`TestUpdateInvocation`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		start := clock.Now(ctx).UTC()

		recorder := newTestRecorderServer()

		token, err := generateInvocationToken(ctx, "inv")
		So(err, ShouldBeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		validDeadline := pbutil.MustTimestampProto(start.Add(day))
		validBigqueryExports := []*pb.BigQueryExport{
			{
				Project: "project",
				Dataset: "dataset",
				Table:   "table1",
				ResultType: &pb.BigQueryExport_TestResults_{
					TestResults: &pb.BigQueryExport_TestResults{},
				},
			},
			{
				Project: "project",
				Dataset: "dataset",
				Table:   "table2",
				ResultType: &pb.BigQueryExport_TestResults_{
					TestResults: &pb.BigQueryExport_TestResults{},
				},
			},
		}

		updateMask := &field_mask.FieldMask{
			Paths: []string{"deadline", "bigquery_exports", "properties", "source_spec"},
		}

		Convey(`invalid request`, func() {
			req := &pb.UpdateInvocationRequest{}
			_, err := recorder.UpdateInvocation(ctx, req)
			So(err, ShouldHaveAppStatus, codes.InvalidArgument, `bad request: invocation: name: unspecified`)
		})

		Convey(`no invocation`, func() {
			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:            "invocations/inv",
					Deadline:        validDeadline,
					BigqueryExports: validBigqueryExports,
				},
				UpdateMask: updateMask,
			}
			_, err := recorder.UpdateInvocation(ctx, req)
			So(err, ShouldHaveAppStatus, codes.NotFound, `invocations/inv not found`)
		})

		// Insert the invocation.
		testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))

		Convey("e2e", func() {
			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:            "invocations/inv",
					Deadline:        validDeadline,
					BigqueryExports: validBigqueryExports,
					Properties:      testutil.TestProperties(),
					SourceSpec: &pb.SourceSpec{
						Sources: testutil.TestSources(431, 123),
					},
				},
				UpdateMask: updateMask,
			}
			inv, err := recorder.UpdateInvocation(ctx, req)
			So(err, ShouldBeNil)

			expected := &pb.Invocation{
				Name:            "invocations/inv",
				Deadline:        validDeadline,
				BigqueryExports: validBigqueryExports,
				Properties:      testutil.TestProperties(),
				SourceSpec: &pb.SourceSpec{
					// The invocation should be stored and returned
					// normalized.
					Sources: testutil.TestSources(123, 431),
				},
			}
			So(inv.Name, ShouldEqual, expected.Name)
			So(inv.State, ShouldEqual, pb.Invocation_ACTIVE)
			So(inv.Deadline, ShouldResembleProto, expected.Deadline)
			So(inv.Properties, ShouldResembleProto, expected.Properties)
			So(inv.SourceSpec, ShouldResembleProto, expected.SourceSpec)

			// Read from the database.
			actual := &pb.Invocation{
				Name:       expected.Name,
				SourceSpec: &pb.SourceSpec{},
			}
			invID := invocations.ID("inv")
			var compressedProperties spanutil.Compressed
			var compressedSources spanutil.Compressed
			testutil.MustReadRow(ctx, "Invocations", invID.Key(), map[string]any{
				"Deadline":        &actual.Deadline,
				"BigQueryExports": &actual.BigqueryExports,
				"Properties":      &compressedProperties,
				"Sources":         &compressedSources,
				"InheritSources":  &actual.SourceSpec.Inherit,
			})
			actual.Properties = &structpb.Struct{}
			err = proto.Unmarshal(compressedProperties, actual.Properties)
			So(err, ShouldBeNil)
			actual.SourceSpec.Sources = &pb.Sources{}
			err = proto.Unmarshal(compressedSources, actual.SourceSpec.Sources)
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, expected)
		})
	})
}
