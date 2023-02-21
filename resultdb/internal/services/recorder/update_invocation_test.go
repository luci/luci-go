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
		Convey(`empty`, func() {
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{}, now)
			So(err, ShouldErrLike, `invocation: name: unspecified`)
		})

		Convey(`invalid id`, func() {
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{Name: "1"},
			}, now)
			So(err, ShouldErrLike, `invocation: name: does not match`)
		})

		Convey(`empty update mask`, func() {
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{Name: "invocations/inv"},
			}, now)
			So(err, ShouldErrLike, `update_mask: paths is empty`)
		})

		Convey(`unsupported update mask`, func() {
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{Name: "invocations/inv"},
				UpdateMask: &field_mask.FieldMask{
					Paths: []string{"name"},
				},
			}, now)
			So(err, ShouldErrLike, `update_mask: unsupported path "name"`)
		})

		Convey(`invalid deadline`, func() {
			deadline := pbutil.MustTimestampProto(now.Add(-time.Hour))
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:     "invocations/inv",
					Deadline: deadline,
				},
				UpdateMask: &field_mask.FieldMask{
					Paths: []string{"deadline"},
				},
			}, now)
			So(err, ShouldErrLike, `invocation: deadline: must be at least 10 seconds in the future`)
		})

		Convey(`invalid bigquery exports`, func() {
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name: "invocations/inv",
					BigqueryExports: []*pb.BigQueryExport{{
						Project: "project",
						Dataset: "dataset",
						Table:   "table",
						// No ResultType.
					}},
				},
				UpdateMask: &field_mask.FieldMask{
					Paths: []string{"bigquery_exports"},
				},
			}, now)
			So(err, ShouldErrLike, `invocation: bigquery_exports[0]: result_type: unspecified`)
		})

		Convey(`invalid properties`, func() {
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name: "invocations/inv",
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key1": structpb.NewStringValue(strings.Repeat("1", pbutil.MaxSizeProperties)),
						},
					},
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"properties"}},
			}, now)
			So(err, ShouldErrLike, `invocation: properties: exceeds the maximum size of`, `bytes`)
		})

		Convey(`valid deadline`, func() {
			deadline := pbutil.MustTimestampProto(now.Add(time.Hour))
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:     "invocations/inv",
					Deadline: deadline,
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"deadline"}},
			}, now)
			So(err, ShouldBeNil)
		})

		Convey(`valid bigquery exports`, func() {
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name: "invocations/inv",
					BigqueryExports: []*pb.BigQueryExport{{
						Project: "project",
						Dataset: "dataset",
						Table:   "table",
						ResultType: &pb.BigQueryExport_TestResults_{
							TestResults: &pb.BigQueryExport_TestResults{},
						},
					}},
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"bigquery_exports"}},
			}, now)
			So(err, ShouldBeNil)
		})

		Convey(`valid properties`, func() {
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name: "invocations/inv",
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key_1": structpb.NewStringValue("value_1"),
							"key_2": structpb.NewStructValue(&structpb.Struct{
								Fields: map[string]*structpb.Value{
									"child_key": structpb.NewNumberValue(1),
								},
							}),
						},
					},
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"properties"}},
			}, now)
			So(err, ShouldBeNil)
		})

		Convey(`empty bigquery export`, func() {
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:            "invocations/inv",
					BigqueryExports: []*pb.BigQueryExport{},
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"bigquery_exports"}},
			}, now)
			So(err, ShouldBeNil)
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
		validProperties := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"key_1": structpb.NewStringValue("value_1"),
				"key_2": structpb.NewStructValue(&structpb.Struct{
					Fields: map[string]*structpb.Value{
						"child_key": structpb.NewNumberValue(1),
					},
				}),
			},
		}

		updateMask := &field_mask.FieldMask{
			Paths: []string{"deadline", "bigquery_exports", "properties"},
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
					Properties:      validProperties,
				},
				UpdateMask: updateMask,
			}
			_, err := recorder.UpdateInvocation(ctx, req)
			So(err, ShouldHaveAppStatus, codes.NotFound, `invocations/inv not found`)
		})

		// Insert the invocation.
		testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))

		Convey("e2e", func() {
			expected := &pb.Invocation{
				Name:            "invocations/inv",
				Deadline:        validDeadline,
				BigqueryExports: validBigqueryExports,
				Properties:      validProperties,
			}
			req := &pb.UpdateInvocationRequest{
				Invocation: expected,
				UpdateMask: updateMask,
			}
			inv, err := recorder.UpdateInvocation(ctx, req)
			So(err, ShouldBeNil)
			So(inv.Name, ShouldEqual, expected.Name)
			So(inv.State, ShouldEqual, pb.Invocation_ACTIVE)
			So(inv.Deadline, ShouldResembleProto, expected.Deadline)
			So(inv.Properties, ShouldResembleProto, expected.Properties)

			// Read from the database.
			actual := &pb.Invocation{
				Name: expected.Name,
			}
			invID := invocations.ID("inv")
			var compressedProperties spanutil.Compressed
			testutil.MustReadRow(ctx, "Invocations", invID.Key(), map[string]any{
				"Deadline":        &actual.Deadline,
				"BigQueryExports": &actual.BigqueryExports,
				"Properties":      &compressedProperties,
			})
			actual.Properties = &structpb.Struct{}
			err = proto.Unmarshal(compressedProperties, actual.Properties)
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, expected)
		})
	})
}
