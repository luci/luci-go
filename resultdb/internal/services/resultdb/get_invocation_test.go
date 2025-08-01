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

package resultdb

import (
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestValidateGetInvocationRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateGetInvocationRequest`, t, func(t *ftt.Test) {
		t.Run(`Valid`, func(t *ftt.Test) {
			req := &pb.GetInvocationRequest{Name: "invocations/valid_id_0"}
			assert.Loosely(t, validateGetInvocationRequest(req), should.BeNil)
		})

		t.Run(`Invalid name`, func(t *ftt.Test) {
			t.Run(`, missing`, func(t *ftt.Test) {
				req := &pb.GetInvocationRequest{}
				assert.Loosely(t, validateGetInvocationRequest(req), should.ErrLike("name missing"))
			})

			t.Run(`, invalid format`, func(t *ftt.Test) {
				req := &pb.GetInvocationRequest{Name: "bad_name"}
				assert.Loosely(t, validateGetInvocationRequest(req), should.ErrLike("does not match"))
			})
		})
	})
}

func TestGetInvocation(t *testing.T) {
	ftt.Run(`GetInvocation`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermGetInvocation},
			},
		})
		ct := testclock.TestRecentTimeUTC
		deadline := ct.Add(time.Hour)
		srv := newTestResultDBService()

		extendedProperties := testutil.TestInvocationExtendedProperties()
		internalExtendedProperties := &invocationspb.ExtendedProperties{
			ExtendedProperties: extendedProperties,
		}

		t.Run(`Valid`, func(t *ftt.Test) {
			// Insert some Invocations.
			moduleVariant := pbutil.Variant("k1", "v1", "k2", "v2")
			testutil.MustApply(ctx, t,
				insert.Invocation("including", pb.Invocation_ACTIVE, map[string]any{
					"CreateTime":         ct,
					"Deadline":           deadline,
					"Realm":              "testproject:testrealm",
					"Properties":         spanutil.Compress(pbutil.MustMarshal(testutil.TestProperties())),
					"Sources":            spanutil.Compress(pbutil.MustMarshal(testutil.TestSources())),
					"InheritSources":     spanner.NullBool{Valid: true, Bool: true},
					"BaselineId":         "testrealm:testbuilder",
					"ExtendedProperties": spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties)),
					"ModuleName":         "modulename",
					"ModuleScheme":       "gtest",
					"ModuleVariant":      moduleVariant,
					"ModuleVariantHash":  pbutil.VariantHash(moduleVariant),
				}),
				insert.Invocation("included0", pb.Invocation_FINALIZED, nil),
				insert.Invocation("included1", pb.Invocation_FINALIZED, nil),
				insert.Inclusion("including", "included0"),
				insert.Inclusion("including", "included1"),
			)

			// Fetch back the top-level Invocation.
			req := &pb.GetInvocationRequest{Name: "invocations/including"}
			inv, err := srv.GetInvocation(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv, should.Match(&pb.Invocation{
				Name:                "invocations/including",
				State:               pb.Invocation_ACTIVE,
				CreateTime:          pbutil.MustTimestampProto(ct),
				Deadline:            pbutil.MustTimestampProto(deadline),
				IncludedInvocations: []string{"invocations/included0", "invocations/included1"},
				ModuleId: &pb.ModuleIdentifier{
					ModuleName:        "modulename",
					ModuleScheme:      "gtest",
					ModuleVariant:     moduleVariant,
					ModuleVariantHash: pbutil.VariantHash(moduleVariant),
				},
				Realm:      "testproject:testrealm",
				Properties: testutil.TestProperties(),
				SourceSpec: &pb.SourceSpec{
					Sources: testutil.TestSources(),
					Inherit: true,
				},
				BaselineId:             "testrealm:testbuilder",
				ExtendedProperties:     extendedProperties,
				TestResultVariantUnion: &pb.Variant{},
			}))
		})

		t.Run(`Permission denied`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Invocation("secret", pb.Invocation_ACTIVE, map[string]any{
					"Realm": "secretproject:testrealm",
				}),
			)
			req := &pb.GetInvocationRequest{Name: "invocations/secret"}
			_, err := srv.GetInvocation(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.invocations.get"))
		})
	})
}
