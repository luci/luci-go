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
	"go.chromium.org/luci/resultdb/internal/config"
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
		return errors.New("must be at least 10 seconds in the future")

	case d.Sub(now) > maxInvocationDeadlineDuration:
		return errors.Fmt("must be before %dh in the future", int(maxInvocationDeadlineDuration.Hours()))

	default:
		return nil
	}
}

// validateCreateInvocationRequest returns an error if req is determined to be
// invalid.
// It also adds the invocations to be included into the newly
// created invocation to the given IDSet.
func validateCreateInvocationRequest(req *pb.CreateInvocationRequest, cfg *config.CompiledServiceConfig, now time.Time, includedIDs invocations.IDSet) error {
	if err := pbutil.ValidateInvocationID(req.InvocationId); err != nil {
		return errors.Fmt("invocation_id: %w", err)
	}
	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Fmt("request_id: %w", err)
	}

	inv := req.Invocation
	if inv == nil {
		return errors.New("invocation: unspecified")
	}

	if err := pbutil.ValidateStringPairs(inv.GetTags()); err != nil {
		return errors.Fmt("invocation: tags: %w", err)
	}

	if inv.Realm == "" {
		return errors.New("invocation: realm: unspecified")
	}

	if err := realms.ValidateRealmName(inv.Realm, realms.GlobalScope); err != nil {
		return errors.Fmt("invocation: realm: %w", err)
	}

	if inv.GetDeadline() != nil {
		if err := validateInvocationDeadline(inv.Deadline, now); err != nil {
			return errors.Fmt("invocation: deadline: %w", err)
		}
	}

	if !isValidCreateState(inv.State) {
		return errors.Fmt("invocation: state: cannot be created in the state %s", inv.GetState())
	}

	if inv.ModuleId != nil {
		if err := pbutil.ValidateModuleIdentifierForStorage(inv.ModuleId); err != nil {
			return errors.Fmt("invocation: module_id: %w", err)
		}
		if err := validateModuleIdentifierAgainstConfig(inv.ModuleId, cfg); err != nil {
			return errors.Fmt("invocation: module_id: %w", err)
		}
	}

	for i, bqExport := range inv.BigqueryExports {
		if err := pbutil.ValidateBigQueryExport(bqExport); err != nil {
			return errors.Fmt("invocation: bigquery_export[%d]: %w", i, err)
		}
	}

	for i, incInvName := range inv.IncludedInvocations {
		incInvID, err := pbutil.ParseInvocationName(incInvName)
		if err != nil {
			return errors.Fmt("invocation: included_invocations[%d]: invalid included invocation name %q: %w", i, incInvName, err)
		}
		if incInvID == req.InvocationId {
			return errors.Fmt("invocation: included_invocations[%d]: invocation cannot include itself", i)
		}
		includedIDs.Add(invocations.ID(incInvID))
	}

	if err := pbutil.ValidateSourceSpec(inv.SourceSpec); err != nil {
		return errors.Fmt("invocation: source_spec: %w", err)
	}

	if inv.GetBaselineId() != "" {
		if err := pbutil.ValidateBaselineID(inv.BaselineId); err != nil {
			return errors.Fmt("invocation: baseline_id: %w", err)
		}
	}

	if err := pbutil.ValidateInvocationProperties(inv.Properties); err != nil {
		return errors.Fmt("invocation: properties: %w", err)
	}

	// In the current flow, step instructions are populated by UpdateInvocation,
	// instead of CreateInvocation.
	// However, we will also store step instructions if they are passed in during creation.
	if err := pbutil.ValidateInstructions(inv.Instructions); err != nil {
		return errors.Fmt("invocation: instructions: %w", err)
	}

	if err := pbutil.ValidateInvocationExtendedProperties(inv.ExtendedProperties); err != nil {
		return errors.Fmt("invocation: extended_properties: %w", err)
	}

	return nil
}

func verifyCreateInvocationPermissions(ctx context.Context, in *pb.CreateInvocationRequest) error {
	inv := in.Invocation
	if inv == nil {
		return appstatus.BadRequest(errors.New("invocation: unspecified"))
	}

	realm := inv.Realm
	if realm == "" {
		return appstatus.BadRequest(errors.New("invocation: realm: unspecified"))
	}
	if err := realms.ValidateRealmName(realm, realms.GlobalScope); err != nil {
		return appstatus.BadRequest(errors.Fmt("invocation: realm: %w", err))
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
	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}

	includedInvs := make(invocations.IDSet)
	if err := validateCreateInvocationRequest(in, cfg, now, includedInvs); err != nil {
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
