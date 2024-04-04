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
	"slices"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// validateUpdateInvocationRequest returns non-nil error if req is invalid.
func validateUpdateInvocationRequest(req *pb.UpdateInvocationRequest, now time.Time) error {
	if err := pbutil.ValidateInvocationName(req.Invocation.GetName()); err != nil {
		return errors.Annotate(err, "invocation: name").Err()
	}

	if len(req.UpdateMask.GetPaths()) == 0 {
		return errors.Reason("update_mask: paths is empty").Err()
	}

	for _, path := range req.UpdateMask.GetPaths() {
		switch path {
		// The cases in this switch statement must be synchronized with a
		// similar switch statement in UpdateInvocation implementation.

		case "deadline":
			if err := validateInvocationDeadline(req.Invocation.GetDeadline(), now); err != nil {
				return errors.Annotate(err, "invocation: deadline").Err()
			}

		case "bigquery_exports":
			for i, bqExport := range req.Invocation.GetBigqueryExports() {
				if err := pbutil.ValidateBigQueryExport(bqExport); err != nil {
					return errors.Annotate(err, "invocation: bigquery_exports[%d]", i).Err()
				}
			}

		case "properties":
			if err := pbutil.ValidateProperties(req.Invocation.Properties); err != nil {
				return errors.Annotate(err, "invocation: properties").Err()
			}

		case "source_spec":
			if err := pbutil.ValidateSourceSpec(req.Invocation.SourceSpec); err != nil {
				return errors.Annotate(err, "invocation: source_spec").Err()
			}

		case "baseline_id":
			if req.Invocation.BaselineId != "" {
				if err := pbutil.ValidateBaselineID(req.Invocation.BaselineId); err != nil {
					return errors.Annotate(err, "invocation: baseline_id").Err()
				}
			}

		case "realm":
			if req.Invocation.Realm == "" {
				return errors.Annotate(errors.Reason("unspecified").Err(), "invocation: realm").Err()
			}
			if err := realms.ValidateRealmName(req.Invocation.Realm, realms.GlobalScope); err != nil {
				return errors.Annotate(err, "invocation: realm").Err()
			}

		case "test_instruction":
			if err := pbutil.ValidateTestInstruction(req.Invocation.GetTestInstruction()); err != nil {
				return errors.Annotate(err, "invocation: test_instruction").Err()
			}

		case "step_instructions":
			if err := pbutil.ValidateStepInstructions(req.Invocation.GetStepInstructions()); err != nil {
				return errors.Annotate(err, "invocation: step_instructions").Err()
			}

		default:
			return errors.Reason("update_mask: unsupported path %q", path).Err()
		}
	}

	return nil
}

func validateUpdateInvocationPermissions(ctx context.Context, existingRealm string, req *pb.UpdateInvocationRequest) error {
	// Note: Permission to update the invocation itself is verified by
	// checking the update-token, which occurs in mutateInvocation(...).

	realm := existingRealm
	project, _ := realms.Split(existingRealm)

	// If there is a change of realm being attempted, verify it first, as
	// further fields must be validated against this new realm, not the old one.
	if slices.Contains(req.UpdateMask.GetPaths(), "realm") {
		realm = req.Invocation.GetRealm()
		newProject, _ := realms.Split(realm)
		if project != newProject {
			// The new realm should be within the same LUCI Project.
			//
			// This ensures the configured BigQuery exports will still
			// be performed with the same project-scoped service account,
			// and the baseline we are writing to is still the same.
			return appstatus.Errorf(codes.InvalidArgument, `cannot change invocation realm to outside project %q`, project)
		}
		if err := validateUpdateRealmPermissions(ctx, realm); err != nil {
			return err
		}
	}

	for _, path := range req.UpdateMask.GetPaths() {
		switch path {
		case "baseline_id":
			if req.Invocation.BaselineId != "" {
				if err := validateUpdateBaselinePermissions(ctx, realm); err != nil {
					// TODO: Return an error to the caller instead of swallowing the error.
					logging.Warningf(ctx, "Silently swallowing permission error on %s: %s", req.Invocation.Name, err)

					// Silently reset the baseline ID on the invocation instead.
					req.Invocation.BaselineId = ""
				}
			}
		}
	}
	return nil
}

func validateUpdateRealmPermissions(ctx context.Context, newRealm string) error {
	// Instead of updating the realm of an invocation from A to B, we could
	// have defined a new invocation in realm B, and included that invocation
	// in the current invocation.
	//
	// Both activities would result in:
	// - an invocation existing in realm B (with test results + artifacts being
	//   uploaded to that realm).
	// - test results in realm B being included in invocations that include
	//   the current invocation.
	//
	// Based on the principle of "same activity, same risk, same rules",
	// we apply the same permission checks below.

	switch allowed, err := auth.HasPermission(ctx, permCreateInvocation, newRealm, nil); {
	case err != nil:
		return err
	case !allowed:
		return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission to create invocations in realm %q (required to update invocation realm)`, newRealm)
	}

	switch allowed, err := auth.HasPermission(ctx, permIncludeInvocation, newRealm, nil); {
	case err != nil:
		return err
	case !allowed:
		return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission to include invocations in realm %q (required to update invocation realm)`, newRealm)
	}
	return nil
}

func validateUpdateBaselinePermissions(ctx context.Context, realm string) error {
	// The baseline is a project-scoped resource, so we should check the
	// realm <project>:@project.
	project, _ := realms.Split(realm)
	projectRealm := realms.Join(project, realms.ProjectRealm)

	switch allowed, err := auth.HasPermission(ctx, permPutBaseline, projectRealm, nil); {
	case err != nil:
		return err
	case !allowed:
		return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission to write to test baseline in realm %s`, projectRealm)
	}
	return nil
}

// UpdateInvocation implements pb.RecorderServer.
func (s *recorderServer) UpdateInvocation(ctx context.Context, in *pb.UpdateInvocationRequest) (*pb.Invocation, error) {
	if err := validateUpdateInvocationRequest(in, clock.Now(ctx).UTC()); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	invID := invocations.MustParseName(in.Invocation.Name)

	var ret *pb.Invocation
	err := mutateInvocation(ctx, invID, func(ctx context.Context) error {
		var err error
		if ret, err = invocations.Read(ctx, invID); err != nil {
			return err
		}
		// Perform validation and permission checks in the same transaction
		// as the update, to prevent the possibility of permission check bypass
		// (TOC/TOU bug) in the event of an update-update race.
		if err := validateUpdateInvocationPermissions(ctx, ret.Realm, in); err != nil {
			return err
		}

		values := map[string]any{
			"InvocationId": invID,
		}

		for _, path := range in.UpdateMask.Paths {
			switch path {
			// The cases in this switch statement must be synchronized with a
			// similar switch statement in validateUpdateInvocationRequest.

			case "deadline":
				deadlne := in.Invocation.Deadline
				values["Deadline"] = deadlne
				ret.Deadline = deadlne

			case "bigquery_exports":
				bqExports := in.Invocation.BigqueryExports
				values["BigQueryExports"] = bqExports
				ret.BigqueryExports = bqExports

			case "properties":
				values["Properties"] = spanutil.Compressed(pbutil.MustMarshal(in.Invocation.Properties))
				ret.Properties = in.Invocation.Properties

			case "source_spec":
				included, err := invocations.ReadIncluded(ctx, invID)
				if err != nil {
					logging.Warningf(ctx, "Instrumentation for b/332787707: failed to read included invocations: %v", err)
				}
				if len(included) > 0 {
					example := included.Names()[0]
					logging.Warningf(ctx, "Instrumentation for b/332787707: invocation %v had children like %v before sources were updated", invID, example)
				}

				// Store any gerrit changes in normalised form.
				pbutil.SortGerritChanges(in.Invocation.SourceSpec.GetSources().GetChangelists())
				values["InheritSources"] = spanner.NullBool{Valid: in.Invocation.SourceSpec != nil, Bool: in.Invocation.SourceSpec.GetInherit()}
				values["Sources"] = spanutil.Compressed(pbutil.MustMarshal(in.Invocation.SourceSpec.GetSources()))
				ret.SourceSpec = in.Invocation.SourceSpec

			case "baseline_id":
				baselineID := in.Invocation.BaselineId
				values["BaselineId"] = baselineID
				ret.BaselineId = baselineID

			case "realm":
				realm := in.Invocation.Realm
				values["Realm"] = realm
				ret.Realm = realm

			case "test_instruction":
				values["TestInstruction"] = spanutil.Compressed(pbutil.MustMarshal(in.Invocation.GetTestInstruction()))
				ret.TestInstruction = in.Invocation.GetTestInstruction()

			case "step_instructions":
				values["StepInstructions"] = spanutil.Compressed(pbutil.MustMarshal(in.Invocation.GetStepInstructions()))
				ret.StepInstructions = in.Invocation.GetStepInstructions()

			default:
				panic("impossible")
			}
		}

		span.BufferWrite(ctx, spanutil.UpdateMap("Invocations", values))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}
