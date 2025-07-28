// Copyright 2025 The LUCI Authors.
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
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tokens"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// DelegateWorkUnitInclusion implements pb.RecorderServer.
func (s *recorderServer) DelegateWorkUnitInclusion(ctx context.Context, in *pb.DelegateWorkUnitInclusionRequest) (*pb.DelegateWorkUnitInclusionResponse, error) {
	if err := verifyDelegateWorkUnitInclusionPermissions(ctx, in); err != nil {
		// Error is already annotated with correct appstatus code.
		return nil, err
	}

	wuID := workunits.MustParseName(in.WorkUnit)

	// Check the work unit is still active.
	state, err := workunits.ReadState(span.Single(ctx), wuID)
	if state != pb.WorkUnit_ACTIVE {
		return nil, appstatus.Errorf(codes.FailedPrecondition, "work unit %q is not active", wuID.Name())
	}

	token, err := generateWorkUnitInclusionToken(ctx, wuID, in.Realm)
	if err != nil {
		// Internal server error.
		return nil, err
	}

	// Return an empty response with the inclusion-token in a response header.
	md := metadata.MD{}
	md.Set(pb.InclusionTokenMetadataKey, token)
	prpc.SetHeader(ctx, md)
	return &pb.DelegateWorkUnitInclusionResponse{}, nil
}

func verifyDelegateWorkUnitInclusionPermissions(ctx context.Context, req *pb.DelegateWorkUnitInclusionRequest) error {
	// Verify we have the update token for the work unit specified in the request.
	rootInvocationID, workUnitID, err := pbutil.ParseWorkUnitName(req.WorkUnit)
	if err != nil {
		return appstatus.BadRequest(errors.Fmt("work_unit: %w", err))
	}

	// Obtain the update token supplied in the request.
	token, err := extractUpdateToken(ctx)
	if err != nil {
		return err
	}

	if err := validateWorkUnitUpdateToken(ctx, token, workunits.ID{
		RootInvocationID: rootinvocations.ID(rootInvocationID),
		WorkUnitID:       workUnitID,
	}); err != nil {
		return appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}

	// Check we have permission to include work units of a given realm into a root invocation (of possibly different
	// realm). This permission authorises the "risk to realm data confidentiality" posed by work unit inclusion,
	// because data in this included work unit may be implicitly shared with readers of the root invocation.
	realm := req.Realm
	if realm == "" {
		return appstatus.BadRequest(errors.New("realm: unspecified"))
	}
	if err := realms.ValidateRealmName(realm, realms.GlobalScope); err != nil {
		return appstatus.BadRequest(errors.Fmt("realm: %w", err))
	}

	if allowed, err := auth.HasPermission(ctx, permIncludeWorkUnit, realm, nil); err != nil {
		return err
	} else if !allowed {
		return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %q in realm %q`, permIncludeWorkUnit, realm)
	}
	return nil
}

// workUnitInclusionTokenKind generates and validates tokens issued to authorize
// including work units of a given realm into a given work unit.
var workUnitInclusionTokenKind = tokens.TokenKind{
	Algo: tokens.TokenAlgoHmacSHA256,
	// The actual secret is derived by mixing the root secret with this key.
	SecretKey: "work_unit_inclusion_token_secret",
	Version:   1,
}

// generateWorkUnitInclusionToken generates an inclusion token for a given work unit.
func generateWorkUnitInclusionToken(ctx context.Context, id workunits.ID, realm string) (string, error) {
	// The token should last at least as long as a work unit is allowed to remain active.
	// Add one day to give some margin, so that "invocation is not active" errors
	// will occur for a while before invalid token errors.
	return workUnitInclusionTokenKind.Generate(ctx, []byte(workUnitInclusionTokenState(id, realm)), nil, maxDeadlineDuration+day)
}

func workUnitInclusionTokenState(id workunits.ID, realm string) string {
	// Quote to ensure state unambiguously represents the three source fields,
	// even in the case any of the fields contain a ';'.
	return fmt.Sprintf("%q;%q;%q", id.RootInvocationID, id.WorkUnitID, realm)
}

// validateWorkUnitInclusionToken validates an inclusion token for a given work unit and realm.
// If the token is valid, including invocations from the given realm into the work unit is authorised.
//
// Returns an error if the token is invalid, nil otherwise.
func validateWorkUnitInclusionToken(ctx context.Context, token string, id workunits.ID, realm string) error {
	_, err := workUnitInclusionTokenKind.Validate(ctx, token, []byte(workUnitInclusionTokenState(id, realm)))
	return err
}
