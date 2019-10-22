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

	"go.chromium.org/luci/resultdb/pbutil"

	"cloud.google.com/go/spanner"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"github.com/golang/protobuf/proto"
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

		Convey(`invalid invocation name`, func() {
			err := validateCreateTestExonerationRequest(&pb.CreateTestExonerationRequest{
				Invocation: "1",
			})
			So(err, ShouldErrLike, `invocation: does not match`)
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

		create := func(requestID, testPath string, variant *pb.VariantDef) (*pb.TestExoneration, error) {
			req := &pb.CreateTestExonerationRequest{
				Invocation: "invocations/inv",
				TestExoneration: &pb.TestExoneration{
					TestVariant: &pb.TestVariant{
						TestPath: testPath,
						Variant:  variant,
					},
				},
				RequestId: requestID,
			}
			return recorder.CreateTestExoneration(ctx, req)
		}

		Convey(`invalid request`, func() {
			_, err := create("", "\x01", nil)
			So(err, ShouldErrLike, `bad request: test_exoneration: test_variant: test_path: does not match`)
			So(grpcutil.Code(err), ShouldEqual, codes.InvalidArgument)
		})

		Convey(`no invocation`, func() {
			_, err := create("", "a", nil)
			So(err, ShouldErrLike, `"invocations/inv" not found`)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
		})

		testutil.MustApply(ctx, testutil.InsertInvocation("inv", pb.Invocation_ACTIVE, token, testclock.TestRecentTimeUTC))

		e2eTest := func(withRequestId bool) {
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

			if withRequestId {
				req.RequestId = "request id"
			}

			actual, err := recorder.CreateTestExoneration(ctx, req)
			So(err, ShouldBeNil)

			invID, exID, err := pbutil.ParseTestExonerationName(actual.Name)
			So(err, ShouldBeNil)
			So(invID, ShouldEqual, "inv")

			if withRequestId {
				So(exID, ShouldEqual, "d:fcc63e142b2aa8429cde3f4e799f05fa8f179e7c57165658e89305e3f6c647580383f4e48a73ed10a58e1d70246eaad194301f053dd453a7b1a078dc2f2d5ae7")
			}

			expected := proto.Clone(req.TestExoneration).(*pb.TestExoneration)
			expected.Name = actual.Name
			So(actual, ShouldResembleProto, expected)

			// Now check the database.
			actual = &pb.TestExoneration{
				TestVariant: &pb.TestVariant{
					Variant: &pb.VariantDef{},
				},
			}
			testutil.MustReadRow(ctx, "TestExonerations", spanner.Key{invID, exID}, map[string]interface{}{
				"TestPath":            &actual.TestVariant.TestPath,
				"VariantDef":          &actual.TestVariant.Variant,
				"ExplanationMarkdown": &actual.ExplanationMarkdown,
			})
			So(actual, ShouldResembleProto, actual)
		}

		Convey(`without request id, e2e`, func() {
			e2eTest(false)
		})
		Convey(`with request id, e2e`, func() {
			e2eTest(true)
		})
	})
}
