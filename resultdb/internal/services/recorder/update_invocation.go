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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
	"go.chromium.org/luci/resultdb/internal/services/exportnotifier"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// validateUpdateInvocationRequestSubmask returns non-nil error if path should
// not have submask, e.g. "deadline.seconds".
func validateUpdateInvocationRequestSubmask(path string, submask *mask.Mask) error {
	if path == "extended_properties" {
		for extPropKey, extPropMask := range submask.Children() {
			if err := pbutil.ValidateInvocationExtendedPropertyKey(extPropKey); err != nil {
				return errors.Annotate(err, "update_mask: extended_properties: key %q", extPropKey).Err()
			}
			if len(extPropMask.Children()) > 0 {
				return errors.Reason("update_mask: extended_properties[%q] should not have any submask", extPropKey).Err()
			}
		}
	} else if len(submask.Children()) > 0 {
		return errors.Reason("update_mask: %q should not have any submask", path).Err()
	}
	return nil
}

// validateUpdateInvocationRequest returns non-nil error if req is invalid.
func validateUpdateInvocationRequest(req *pb.UpdateInvocationRequest, now time.Time) error {
	if err := pbutil.ValidateInvocationName(req.Invocation.GetName()); err != nil {
		return errors.Annotate(err, "invocation: name").Err()
	}

	if len(req.UpdateMask.GetPaths()) == 0 {
		return errors.Reason("update_mask: paths is empty").Err()
	}

	updateMask, err := mask.FromFieldMask(req.UpdateMask, req.Invocation, mask.AdvancedSemantics(), mask.ForUpdate())
	if err != nil {
		return errors.Annotate(err, "update_mask").Err()
	}
	for path, submask := range updateMask.Children() {
		if err := validateUpdateInvocationRequestSubmask(path, submask); err != nil {
			return err
		}
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
			if err := pbutil.ValidateInvocationProperties(req.Invocation.Properties); err != nil {
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

		case "instructions":
			if err := pbutil.ValidateInstructions(req.Invocation.GetInstructions()); err != nil {
				return errors.Annotate(err, "invocation: instructions").Err()
			}

		case "is_source_spec_final":
			// Either true or false is OK for this first pass validation.
			// However, later we must validate that if the field is true,
			// it is not being set to false.

		case "extended_properties":
			if err := pbutil.ValidateInvocationExtendedProperties(req.Invocation.GetExtendedProperties()); err != nil {
				return errors.Annotate(err, "invocation: extended_properties").Err()
			}

		case "tags":
			if err := pbutil.ValidateStringPairs(req.Invocation.GetTags()); err != nil {
				return errors.Annotate(err, "invocation: tags").Err()
			}

		case "state":
			// If "state" is set, we only allow "FINALIZING" or "ACTIVE" state.
			// Setting to "FINALIZING" will trigger the finalization process.
			// Setting to "ACTIVE" is a no-op.
			if req.Invocation.State != pb.Invocation_FINALIZING && req.Invocation.State != pb.Invocation_ACTIVE {
				return errors.Reason("invocation: state: must be FINALIZING or ACTIVE").Err()
			}

		default:
			return errors.Reason("update_mask: unsupported path %q", path).Err()
		}
	}

	return nil
}

func validateUpdateInvocationPermissions(ctx context.Context, existing *pb.Invocation, req *pb.UpdateInvocationRequest) error {
	// Note: Permission to update the invocation itself is verified by
	// checking the update-token, which occurs in mutateInvocation(...).

	realm := existing.Realm
	project, _ := realms.Split(existing.Realm)

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
		case "bigquery_exports":
			if !isBigQueryExportsEqual(req.Invocation.BigqueryExports, existing.BigqueryExports) {
				// Configuring BigQuery exports indirectly grants use of the
				// LUCI project-scoped service account to write to a BigQuery table.

				// Check permission against the root realm <project>:@root as the
				// project-scoped service account is project-scoped resource.
				// We can change this to check realm @project in future but this
				// currently generates a bunch of nuisance warning messages as
				// projects typically do not define a realm '@project' so it
				// falls back to '@root' anyway.
				rootRealm := realms.Join(project, realms.RootRealm)

				switch allowed, err := auth.HasPermission(ctx, permExportToBigQuery, rootRealm, nil); {
				case err != nil:
					return err
				case !allowed:
					return appstatus.Errorf(codes.PermissionDenied, `updater does not have permission to set bigquery exports in realm %q`, rootRealm)
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
	var shouldFinalizeInvocation bool

	var ret *pb.Invocation
	commitTimestamp, err := mutateInvocation(ctx, invID, func(ctx context.Context) error {
		shouldFinalizeInvocation = false
		var err error
		if ret, err = invocations.Read(ctx, invID, invocations.AllFields); err != nil {
			return err
		}
		// Perform validation and permission checks in the same transaction
		// as the update, to prevent the possibility of permission check bypass
		// (TOC/TOU bug) in the event of an update-update race.
		if err := validateUpdateInvocationPermissions(ctx, ret, in); err != nil {
			return err
		}

		values := map[string]any{
			"InvocationId": invID,
		}

		// Capture whether sources were final at the start of processing
		// this request. It should be valid to set both sources and
		// sources final in the same request, regardless of the order
		// the fields are mentioned in the update mask.
		wasSourcesFinal := ret.IsSourceSpecFinal

		updateMask, err := mask.FromFieldMask(in.UpdateMask, in.Invocation, mask.AdvancedSemantics(), mask.ForUpdate())
		if err != nil {
			return errors.Annotate(err, "update_mask").Err()
		}
		for path, submask := range updateMask.Children() {
			switch path {
			// The cases in this switch statement must be synchronized with a
			// similar switch statement in validateUpdateInvocationRequest.

			case "deadline":
				deadline := in.Invocation.Deadline
				values["Deadline"] = deadline
				ret.Deadline = deadline

			case "bigquery_exports":
				if !isBigQueryExportsEqual(in.Invocation.BigqueryExports, ret.BigqueryExports) {
					bqExports := in.Invocation.BigqueryExports
					values["BigQueryExports"] = bqExports
					ret.BigqueryExports = bqExports
				}

			case "properties":
				values["Properties"] = spanutil.Compressed(pbutil.MustMarshal(in.Invocation.Properties))
				ret.Properties = in.Invocation.Properties

			case "tags":
				values["Tags"] = in.Invocation.Tags
				ret.Tags = in.Invocation.Tags

			case "source_spec":
				// Are we setting the field to a value other than its current value?
				updateSources := !proto.Equal(ret.SourceSpec, in.Invocation.SourceSpec)
				if updateSources {
					if wasSourcesFinal {
						return appstatus.BadRequest(errors.Reason("invocation: source_spec: cannot modify already finalized sources").Err())
					}

					// Store any gerrit changes in normalised form.
					pbutil.SortGerritChanges(in.Invocation.SourceSpec.GetSources().GetChangelists())
					values["InheritSources"] = spanner.NullBool{Valid: in.Invocation.SourceSpec != nil, Bool: in.Invocation.SourceSpec.GetInherit()}
					values["Sources"] = spanutil.Compressed(pbutil.MustMarshal(in.Invocation.SourceSpec.GetSources()))
					ret.SourceSpec = in.Invocation.SourceSpec
				}

			case "is_source_spec_final":
				if ret.IsSourceSpecFinal != in.Invocation.IsSourceSpecFinal {
					if !in.Invocation.IsSourceSpecFinal {
						return appstatus.BadRequest(errors.Reason("invocation: is_source_spec_final: cannot unfinalize already finalized sources").Err())
					}

					values["IsSourceSpecFinal"] = spanner.NullBool{Valid: in.Invocation.IsSourceSpecFinal, Bool: in.Invocation.IsSourceSpecFinal}
					ret.IsSourceSpecFinal = in.Invocation.IsSourceSpecFinal

					// Finalizing sources on this invocation also finalizes the sources
					// included invocations are eligible to inherit from this invocation.
					// Run export notifier to propogate this information and send
					// notifications as appropriate.
					exportnotifier.EnqueueTask(ctx, &taskspb.RunExportNotifications{
						InvocationId: string(invID),
					})
				}

			case "baseline_id":
				baselineID := in.Invocation.BaselineId
				values["BaselineId"] = baselineID
				ret.BaselineId = baselineID

			case "realm":
				if in.Invocation.Realm != ret.Realm {
					if ret.IsExportRoot {
						// For ResultDB export to be useful, we must provide both the
						// realm of the root invocation as well as the realm of the
						// immediate invocation a test result was uploaded to.
						// This is because both can contribute to the ACLing of a
						// result, and because the project of the root invocation
						// dictates the project results are exported to.
						// To make low-latency exports possible, we fix the realm
						// of the root invocation from time of its creation.
						return appstatus.BadRequest(errors.Reason("invocation: realm: cannot change realm of an invocation that is an export root").Err())
					}
					realm := in.Invocation.Realm
					values["Realm"] = realm
					ret.Realm = realm
				}

			case "instructions":
				ins := instructionutil.RemoveInstructionsName(in.Invocation.GetInstructions())
				values["Instructions"] = spanutil.Compressed(pbutil.MustMarshal(ins))
				ret.Instructions = instructionutil.InstructionsWithNames(in.Invocation.GetInstructions(), string(invID))

			case "extended_properties":
				extendedProperties := in.Invocation.GetExtendedProperties()
				if len(submask.Children()) > 0 {
					// Init if nil map
					if ret.ExtendedProperties == nil {
						ret.ExtendedProperties = make(map[string]*structpb.Struct)
					}
					// If the update_mask has masks like "extended_properties.some_key".
					for extPropKey := range submask.Children() {
						if _, exist := extendedProperties[extPropKey]; exist {
							// Add or update if extPropKey exists in extendedProperties
							ret.ExtendedProperties[extPropKey] = extendedProperties[extPropKey]
						} else {
							// Delete if does not exist
							delete(ret.ExtendedProperties, extPropKey)
						}
					}
				} else {
					ret.ExtendedProperties = extendedProperties
				}
				// One more validation to ensure the size is within the limit.
				if err := pbutil.ValidateInvocationExtendedProperties(ret.ExtendedProperties); err != nil {
					return appstatus.BadRequest(errors.Annotate(err, "invocation: extended_properties").Err())
				}
				internalExtendedProperties := &invocationspb.ExtendedProperties{
					ExtendedProperties: ret.ExtendedProperties,
				}
				values["ExtendedProperties"] = spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties))
			case "state":
				// In the case of ACTIVE, it should be a No-op.
				if in.Invocation.State == pb.Invocation_FINALIZING {
					shouldFinalizeInvocation = true
					values["State"] = pb.Invocation_FINALIZING
					ret.State = pb.Invocation_FINALIZING
					values["FinalizeStartTime"] = spanner.CommitTimestamp
				}
			default:
				panic("impossible")
			}
		}

		span.BufferWrite(ctx, spanutil.UpdateMap("Invocations", values))
		if shouldFinalizeInvocation {
			tasks.StartInvocationFinalization(ctx, invID, false)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	if shouldFinalizeInvocation {
		ret.FinalizeStartTime = timestamppb.New(commitTimestamp)
	}
	return ret, nil
}

func isBigQueryExportsEqual(a, b []*pb.BigQueryExport) bool {
	// As a and b are slices, they cannot be passed to proto.Equal directly.
	// Wrap them an invocation container.
	aInv := &pb.Invocation{BigqueryExports: a}
	bInv := &pb.Invocation{BigqueryExports: b}
	return proto.Equal(aInv, bInv)
}
