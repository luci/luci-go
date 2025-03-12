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
	"context"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const trustedCreatorGroup = "luci-resultdb-trusted-invocation-creators"

// TestMagicOverdueDeadlineUnixSecs is a magic value used by tests to set an
// invocation's deadline in the past.
const TestMagicOverdueDeadlineUnixSecs = 904924800

// isValidCreateState returns false if invocations cannot be created in the
// given state `s`.
func isValidCreateState(s pb.Invocation_State) bool {
	switch s {
	default:
		return false
	case pb.Invocation_STATE_UNSPECIFIED:
	case pb.Invocation_ACTIVE:
	case pb.Invocation_FINALIZING:
	}
	return true
}

// validateInvocationDeadline returns a non-nil error if deadline is invalid.
func validateInvocationDeadline(deadline *timestamppb.Timestamp, now time.Time) error {
	internal.AssertUTC(now)
	switch d, err := ptypes.Timestamp(deadline); {
	case err != nil:
		return err

	case deadline.GetSeconds() == TestMagicOverdueDeadlineUnixSecs && deadline.GetNanos() == 0:
		return nil

	case d.Sub(now) < 10*time.Second:
		return errors.Reason("must be at least 10 seconds in the future").Err()

	case d.Sub(now) > maxInvocationDeadlineDuration:
		return errors.Reason("must be before %dh in the future", int(maxInvocationDeadlineDuration.Hours())).Err()

	default:
		return nil
	}
}

// validateCreateInvocationRequest returns an error if req is determined to be
// invalid.
// It also adds the invocations to be included into the newly
// created invocation to the given IDSet.
func validateCreateInvocationRequest(req *pb.CreateInvocationRequest, now time.Time, includedIDs invocations.IDSet) error {
	if err := pbutil.ValidateInvocationID(req.InvocationId); err != nil {
		return errors.Annotate(err, "invocation_id").Err()
	}
	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Annotate(err, "request_id").Err()
	}

	inv := req.Invocation
	if inv == nil {
		return errors.Annotate(errors.Reason("unspecified").Err(), "invocation").Err()
	}

	if err := pbutil.ValidateStringPairs(inv.GetTags()); err != nil {
		return errors.Annotate(err, "invocation: tags").Err()
	}

	if inv.Realm == "" {
		return errors.Annotate(errors.Reason("unspecified").Err(), "invocation: realm").Err()
	}

	if err := realms.ValidateRealmName(inv.Realm, realms.GlobalScope); err != nil {
		return errors.Annotate(err, "invocation: realm").Err()
	}

	if inv.GetDeadline() != nil {
		if err := validateInvocationDeadline(inv.Deadline, now); err != nil {
			return errors.Annotate(err, "invocation: deadline").Err()
		}
	}

	if !isValidCreateState(inv.GetState()) {
		return errors.Reason("invocation: state: cannot be created in the state %s", inv.GetState()).Err()
	}

	for i, bqExport := range inv.GetBigqueryExports() {
		if err := pbutil.ValidateBigQueryExport(bqExport); err != nil {
			return errors.Annotate(err, "bigquery_export[%d]", i).Err()
		}
	}

	for i, incInvName := range inv.GetIncludedInvocations() {
		incInvID, err := pbutil.ParseInvocationName(incInvName)
		if err != nil {
			return errors.Annotate(err, "included_invocations[%d]: invalid included invocation name %q", i, incInvName).Err()
		}
		if incInvID == req.InvocationId {
			return errors.Reason("included_invocations[%d]: invocation cannot include itself", i).Err()
		}
		includedIDs.Add(invocations.ID(incInvID))
	}

	if err := pbutil.ValidateSourceSpec(inv.GetSourceSpec()); err != nil {
		return errors.Annotate(err, "source_spec").Err()
	}

	if inv.GetBaselineId() != "" {
		if err := pbutil.ValidateBaselineID(inv.GetBaselineId()); err != nil {
			return errors.Annotate(err, "invocation: baseline_id").Err()
		}
	}

	if err := pbutil.ValidateInvocationProperties(req.Invocation.GetProperties()); err != nil {
		return errors.Annotate(err, "properties").Err()
	}

	// In the current flow, step instructions are populated by UpdateInvocation,
	// instead of CreateInvocation.
	// However, we will also store step instructions if they are passed in during creation.
	if err := pbutil.ValidateInstructions(req.Invocation.GetInstructions()); err != nil {
		return errors.Annotate(err, "instructions").Err()
	}

	if err := pbutil.ValidateInvocationExtendedProperties(req.Invocation.GetExtendedProperties()); err != nil {
		return errors.Annotate(err, "extended_properties").Err()
	}

	return nil
}

func verifyCreateInvocationPermissions(ctx context.Context, in *pb.CreateInvocationRequest) error {
	inv := in.Invocation
	if inv == nil {
		return appstatus.BadRequest(errors.Annotate(errors.Reason("unspecified").Err(), "invocation").Err())
	}

	realm := inv.Realm
	if realm == "" {
		return appstatus.BadRequest(errors.Annotate(errors.Reason("unspecified").Err(), "invocation: realm").Err())
	}
	if err := realms.ValidateRealmName(realm, realms.GlobalScope); err != nil {
		return appstatus.BadRequest(errors.Annotate(err, "invocation: realm").Err())
	}

	switch allowed, err := auth.HasPermission(ctx, permCreateInvocation, realm, nil); {
	case err != nil:
		return err
	case !allowed:
		return appstatus.Errorf(codes.PermissionDenied, `creator does not have permission to create invocations in realm %q`, realm)
	}

	if !strings.HasPrefix(in.InvocationId, "u-") {
		// Ensure the integrity of invocation names with reserved IDs
		// by ensuring the caller is a trusted service.

		// After creation, the caller may attempt to update the invocation
		// to another subrealm of the same project.
		// Check we are trusted to create invocations with reserved ID
		// in <project>:@root to cover all realms this invocation could
		// eventually end up in.

		// Find the root realm <project>:@root. If the caller has the permission
		// in this realm, it has permission in every realm of the project.
		project, _ := realms.Split(realm)
		rootRealm := realms.Join(project, realms.RootRealm)
		allowed, err := checkPermissionOrGroupMember(ctx, rootRealm, permCreateWithReservedID, trustedCreatorGroup)
		if err != nil {
			return err
		}
		if !allowed {
			return appstatus.Errorf(codes.PermissionDenied, `only invocations created by trusted systems may have id not starting with "u-"; please generate "u-{GUID}" or reach out to ResultDB owners`)
		}
	}

	if inv.GetIsExportRoot() {
		// Export roots cannot have their realm changed after creation,
		// so we do not need to check the project's root realm, like the
		// other cases.
		allowed, err := checkPermissionOrGroupMember(ctx, realm, permSetExportRoot, trustedCreatorGroup)
		if err != nil {
			return err
		}
		if !allowed {
			return appstatus.Errorf(codes.PermissionDenied, `creator does not have permission to set export roots in realm %q`, realm)
		}
	}

	if len(inv.GetBigqueryExports()) > 0 {
		// Configuring BigQuery exports indirectly grants use of the
		// LUCI project-scoped service account to write to a BigQuery table.

		// Check permission against the root realm <project>:@root as the
		// project-scoped service account is project-scoped resource.
		//
		// We can change this to check realm @project in future but this
		// currently generates a bunch of nuisance warning messages as
		// projects typically do not define a realm '@project' so it
		// falls back to '@root' anyway.
		project, _ := realms.Split(realm)
		rootRealm := realms.Join(project, realms.RootRealm)

		switch allowed, err := auth.HasPermission(ctx, permExportToBigQuery, rootRealm, nil); {
		case err != nil:
			return err
		case !allowed:
			return appstatus.Errorf(codes.PermissionDenied, `creator does not have permission to set bigquery exports in realm %q`, rootRealm)
		}
	}

	if inv.GetProducerResource() != "" {
		// Ensure the integrity of the producer resource by ensuring the
		// caller is a trusted service.

		// After creation, the caller may attempt to update the invocation
		// to another subrealm of the project.
		// Check we are trusted to set producer resource in <project>:@root
		// to cover all realms this invocation could eventually end up in.

		// Find the root realm <project>:@root. If the caller has the permission
		// in this realm, it has permission in every realm of the project.
		project, _ := realms.Split(realm)
		rootRealm := realms.Join(project, realms.RootRealm)
		allowed, err := checkPermissionOrGroupMember(ctx, rootRealm, permSetProducerResource, trustedCreatorGroup)
		if err != nil {
			return err
		}
		if !allowed {
			return appstatus.Errorf(codes.PermissionDenied, `only invocations created by trusted system may have a populated producer_resource field`)
		}
	}

	if inv.BaselineId != "" {
		// Baselines are project-scoped resources. Find the project-scoped
		// realm <project>:@project and check authorisation to write to it.
		project, _ := realms.Split(realm)
		projectRealm := realms.Join(project, realms.ProjectRealm)

		switch allowed, err := auth.HasPermission(ctx, permPutBaseline, projectRealm, nil); {
		case err != nil:
			return err
		case !allowed:
			return appstatus.Errorf(codes.PermissionDenied, `creator does not have permission to write to test baseline in realm %q`, projectRealm)
		}
	}

	return nil
}

// checkPermissionOrGroupMember returns true if the caller has permission in realm
// or is a member of the group.
func checkPermissionOrGroupMember(ctx context.Context, realm string, permission realms.Permission, group string) (bool, error) {
	switch allowed, err := auth.HasPermission(ctx, permission, realm, nil); {
	case err != nil:
		return false, err
	case !allowed:
		switch isMember, err := auth.IsMember(ctx, group); {
		case err != nil:
			return false, err
		case !isMember:
			return false, nil
		}
	}
	return true, nil
}

// CreateInvocation implements pb.RecorderServer.
func (s *recorderServer) CreateInvocation(ctx context.Context, in *pb.CreateInvocationRequest) (*pb.Invocation, error) {
	now := clock.Now(ctx).UTC()

	if err := verifyCreateInvocationPermissions(ctx, in); err != nil {
		return nil, err
	}

	includedInvs := make(invocations.IDSet)
	if err := validateCreateInvocationRequest(in, now, includedInvs); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	if err := permissions.VerifyInvocations(span.Single(ctx), includedInvs, permIncludeInvocation); err != nil {
		return nil, err
	}

	invs, tokens, err := s.createInvocations(ctx, []*pb.CreateInvocationRequest{in}, in.RequestId, now, invocations.NewIDSet(invocations.ID(in.InvocationId)))
	if err != nil {
		return nil, err
	}
	if len(invs) != 1 || len(tokens) != 1 {
		panic("createInvocations did not return either an error or a valid invocation/token pair")
	}
	md := metadata.MD{}
	md.Set(pb.UpdateTokenMetadataKey, tokens...)
	prpc.SetHeader(ctx, md)
	return invs[0], nil
}

func invocationAlreadyExists(id invocations.ID) error {
	return appstatus.Errorf(codes.AlreadyExists, "%s already exists", id.Name())
}
