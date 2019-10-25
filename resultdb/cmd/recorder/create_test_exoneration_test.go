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

package main

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateCreateTestExonerationRequest(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateCreateTestExonerationRequest`, t, func() {
		Convey(`empty`, func() {
			err := validateCreateTestExonerationRequest(&pb.CreateTestExonerationRequest{})
			So(err, ShouldErrLike, `invocation: unspecified`)
		})

		Convey(`invalid test variant`, func() {
			err := validateCreateTestExonerationRequest(&pb.CreateTestExonerationRequest{
				Invocation: "invocations/inv",
				TestExoneration: &pb.TestExoneration{
					TestVariant: &pb.TestVariant{TestPath: "\x01"},
				},
			})
			So(err, ShouldErrLike, `test_exoneration: test_variant: test_path: does not match`)
		})

		Convey(`valid`, func() {
			err := validateCreateTestExonerationRequest(&pb.CreateTestExonerationRequest{
				Invocation: "invocations/inv",
				TestExoneration: &pb.TestExoneration{
					TestVariant: &pb.TestVariant{
						TestPath: "gn://ab/cd.ef",
						Variant: &pb.VariantDef{
							Def: map[string]string{
								"a/b": "1",
								"c":   "2",
							},
						},
					},
				},
			})
			So(err, ShouldBeNil)
		})
	})
}

func TestCreateTestExoneration(t *testing.T) {
	Convey(`TestCreateTestExoneration`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		// Init auth state.
		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{
			Identity: identity.AnonymousIdentity,
		}
		ctx = auth.WithState(ctx, authState)

		recorder := NewRecorderServer()

		const token = "update token"
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(updateTokenMetadataKey, token))

		Convey(`invalid request`, func() {
			req := &pb.CreateTestExonerationRequest{
				Invocation: "invocations/inv",
				TestExoneration: &pb.TestExoneration{
					TestVariant: &pb.TestVariant{
						TestPath: "\x01",
					},
				},
			}
			_, err := recorder.CreateTestExoneration(ctx, req)
			So(err, ShouldErrLike, `bad request: test_exoneration: test_variant: test_path: does not match`)
			So(grpcutil.Code(err), ShouldEqual, codes.InvalidArgument)
		})

		Convey(`no invocation`, func() {
			req := &pb.CreateTestExonerationRequest{
				Invocation: "invocations/inv",
				TestExoneration: &pb.TestExoneration{
					TestVariant: &pb.TestVariant{
						TestPath: "a",
					},
				},
			}
			_, err := recorder.CreateTestExoneration(ctx, req)
			So(err, ShouldErrLike, `"invocations/inv" not found`)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
		})

		// Insert the invocation.
		testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, token, testclock.TestRecentTimeUTC))

		e2eTest := func(withRequestID bool) {
			req := &pb.CreateTestExonerationRequest{
				Invocation: "invocations/inv",
				TestExoneration: &pb.TestExoneration{
					TestVariant: &pb.TestVariant{
						TestPath: "a",
						Variant: &pb.VariantDef{
							Def: map[string]string{
								"a": "1",
								"b": "2",
							},
						},
					},
				},
			}

			if withRequestID {
				req.RequestId = "request id"
			}

			res, err := recorder.CreateTestExoneration(ctx, req)
			So(err, ShouldBeNil)

			So(res.ExonerationId, ShouldStartWith, "6408fdc5c36df5df57957e7b230e4b48c55b06ce849303e855d52f1356f2980e:") // hash of the variant
			if withRequestID {
				So(res.ExonerationId, ShouldEqual, "6408fdc5c36df5df57957e7b230e4b48c55b06ce849303e855d52f1356f2980e:d:fcc63e142b2aa8429cde3f4e799f05fa8f179e7c57165658e89305e3f6c647580383f4e48a73ed10a58e1d70246eaad194301f053dd453a7b1a078dc2f2d5ae7")
			}

			expected := proto.Clone(req.TestExoneration).(*pb.TestExoneration)
			proto.Merge(expected, &pb.TestExoneration{
				Name:          pbutil.TestExonerationName("inv", "a", res.ExonerationId),
				ExonerationId: res.ExonerationId,
			})
			So(res, ShouldResembleProto, expected)

			// Now check the database.
			row, err := span.ReadTestExonerationFull(ctx, span.Client(ctx).Single(), "inv", "a", res.ExonerationId)
			So(err, ShouldBeNil)
			So(row.TestVariant.Variant, ShouldResembleProto, expected.TestVariant.Variant)
			So(row.ExplanationMarkdown, ShouldEqual, expected.ExplanationMarkdown)

			if withRequestID {
				// Test idempotency.
				res2, err := recorder.CreateTestExoneration(ctx, req)
				So(err, ShouldBeNil)
				So(res2, ShouldResembleProto, res)
			}
		}

		Convey(`without request id, e2e`, func() {
			e2eTest(false)
		})
		Convey(`with request id, e2e`, func() {
			e2eTest(true)
		})
	})
}
