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
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// maxDeadlineDuration is the maximum amount of time a root invocation or
// work unit can spend in the ACTIVE state before it should be finalized.
const maxDeadlineDuration = 5 * 24 * time.Hour // 5 days

// CreateRootInvocation implements pb.RecorderServer.
func (s *recorderServer) CreateRootInvocation(ctx context.Context, in *pb.CreateRootInvocationRequest) (*pb.CreateRootInvocationResponse, error) {
	// As per https://google.aip.dev/211, check authorisation before
	// validating the request.
	if err := verifyCreateRootInvocationPermissions(ctx, in); err != nil {
		return nil, err
	}
	if err := validateCreateRootInvocationRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	return nil, appstatus.Error(codes.Unimplemented, "not yet implemented")
}

func verifyCreateRootInvocationPermissions(ctx context.Context, req *pb.CreateRootInvocationRequest) error {
	// Only perform validation that is necessary for performing authorisation checks,
	// the full set of validation will occur if these checks pass.
	inv := req.RootInvocation
	if inv == nil {
		return appstatus.BadRequest(errors.New("root_invocation: unspecified"))
	}
	realm := inv.Realm
	if realm == "" {
		return appstatus.BadRequest(errors.New("root_invocation: realm: unspecified"))
	}
	if err := realms.ValidateRealmName(realm, realms.GlobalScope); err != nil {
		return appstatus.BadRequest(errors.Fmt("root_invocation: realm: %w", err))
	}

	// Check we have permission to create the root invocation and its root work unit.
	if allowed, err := auth.HasPermission(ctx, permCreateRootInvocation, realm, nil); err != nil {
		return err
	} else if !allowed {
		return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %q in realm %q`, permCreateRootInvocation, realm)
	}
	if allowed, err := auth.HasPermission(ctx, permCreateWorkUnit, realm, nil); err != nil {
		return err
	} else if !allowed {
		return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %q in realm %q`, permCreateWorkUnit, realm)
	}

	// if an ID not starting with "u-" is specified,
	// resultdb.rootInvocations.createWithReservedID permission is required.
	if !strings.HasPrefix(req.RootInvocationId, "u-") {
		project, _ := realms.Split(realm)
		rootRealm := realms.Join(project, realms.RootRealm)
		allowed, err := checkPermissionOrGroupMember(ctx, rootRealm, permCreateRootInvocationWithReservedID, trustedCreatorGroup)
		if err != nil {
			return err
		}
		if !allowed {
			return appstatus.Errorf(codes.PermissionDenied, `only root invocations created by trusted systems may have id not starting with "u-"; please generate "u-{GUID}" or reach out to ResultDB owners`)
		}
	}

	// if the producer resource is set,
	// resultdb.rootInvocations.setProducerResource permission is required.
	if inv.ProducerResource != "" {
		project, _ := realms.Split(realm)
		rootRealm := realms.Join(project, realms.RootRealm)
		allowed, err := checkPermissionOrGroupMember(ctx, rootRealm, permSetRootInvocationProducerResource, trustedCreatorGroup)
		if err != nil {
			return err
		}
		if !allowed {
			return appstatus.Errorf(codes.PermissionDenied, `only root invocations created by trusted system may have a populated producer_resource field`)
		}
	}

	// if a baseline is set,
	// resultdb.baselines.put permission is required in the ":@project" realm
	// of the LUCI project the root invocation is being created in.
	if inv.BaselineId != "" {
		project, _ := realms.Split(realm)
		projectRealm := realms.Join(project, realms.ProjectRealm)
		if allowed, err := auth.HasPermission(ctx, permPutBaseline, projectRealm, nil); err != nil {
			return err
		} else if !allowed {
			return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission to write to test baseline in realm %q`, projectRealm)
		}
	}

	return nil
}

func validateCreateRootInvocationRequest(req *pb.CreateRootInvocationRequest) error {
	if err := pbutil.ValidateRootInvocationID(req.RootInvocationId); err != nil {
		return errors.Fmt("root_invocation_id: %w", err)
	}
	if err := validateRootInvocationForCreate(req.RootInvocation); err != nil {
		return errors.Fmt("root_invocation: %w", err)
	}
	if err := validateRootWorkUnitForCreate(req.RootWorkUnit); err != nil {
		return errors.Fmt("root_work_unit: %w", err)
	}
	if req.RequestId == "" {
		// Request ID is required to ensure requests are treated idempotently
		// in case of inevitable retries.
		return errors.Fmt("request_id: unspecified (please provide a per-request UUID to ensure idempotence)")
	}
	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Fmt("request_id: %w", err)
	}
	return nil
}

// validateRootInvocationForCreate validates the fields of a pb.RootInvocation message
// for a creation request.
func validateRootInvocationForCreate(inv *pb.RootInvocation) error {
	if inv == nil {
		return errors.New("unspecified")
	}
	// This method attempts to validate fields in the order that they are specified in the proto.

	// Name and RootInvocationId is output only and should be ignored
	// as per https://google.aip.dev/203.

	// Validate state.
	switch inv.State {
	case pb.RootInvocation_STATE_UNSPECIFIED, pb.RootInvocation_ACTIVE, pb.RootInvocation_FINALIZING:
		// Allowed states for creation.
	default:
		return errors.Fmt("state: cannot be created in the state %s", inv.State)
	}

	if inv.Realm == "" {
		return errors.New("realm: unspecified")
	}
	if err := realms.ValidateRealmName(inv.Realm, realms.GlobalScope); err != nil {
		return errors.Fmt("realm: %w", err)
	}

	// CreateTime, Creator, FinalizeStartTime, FinalizeTime are output only and should be ignored
	// as per https://google.aip.dev/203.

	// Validate deadline.
	if inv.Deadline != nil {
		// Using time.Now() for validation, actual commit time will be used for storage.
		assumedCreateTime := time.Now().UTC()
		if err := validateDeadline(inv.Deadline, assumedCreateTime); err != nil {
			return errors.Fmt("deadline: %w", err)
		}
	}

	// Validate producer_resource.
	if inv.ProducerResource != "" {
		if err := pbutil.ValidateFullResourceName(inv.ProducerResource); err != nil {
			return errors.Fmt("producer_resource: %w", err)
		}
	}

	// Validate sources.
	if inv.Sources != nil {
		if err := pbutil.ValidateSources(inv.Sources); err != nil {
			return errors.Fmt("sources: %w", err)
		}
	}

	// Validate tags.
	if err := pbutil.ValidateRootInvocationTags(inv.Tags); err != nil {
		return errors.Fmt("tags: %w", err)
	}

	// Validate properties.
	if inv.Properties != nil {
		if err := pbutil.ValidateRootInvocationProperties(inv.Properties); err != nil {
			return errors.Fmt("properties: %w", err)
		}
	}

	// Validate baseline_id.
	if inv.BaselineId != "" {
		if err := pbutil.ValidateBaselineID(inv.BaselineId); err != nil {
			return errors.Fmt("baseline_id: %w", err)
		}
	}

	return nil
}

// validateRootWorkUnitForCreate validates the fields of a pb.WorkUnit message
// for a creation request within a root invocation.
func validateRootWorkUnitForCreate(wu *pb.WorkUnit) error {
	if wu == nil {
		return errors.New("unspecified")
	}
	// This method attempts to validate fields in the order that they are specified in the proto.

	// Name and WorkUnitId is output only and should be ignored
	// as per https://google.aip.dev/203.

	if wu.State != pb.WorkUnit_STATE_UNSPECIFIED {
		return errors.New("state: must not be set; always inherited from root invocation")
	}
	if wu.Realm != "" {
		return errors.New("realm: must not be set; always inherited from root invocation")
	}

	// CreateTime, Creator, FinalizeStartTime, FinalizeTime are output only and should be ignored
	// as per https://google.aip.dev/203.

	if wu.Deadline != nil {
		return errors.New("deadline: must not be set; always inherited from root invocation")
	}

	// Parent, ChildWorkUnits, ChildInvocations are output only and should be ignored
	// as per https://google.aip.dev/203.

	if wu.ProducerResource != "" {
		return errors.New("producer_resource: must not be set; always inherited from root invocation")
	}
	if err := pbutil.ValidateWorkUnitTags(wu.Tags); err != nil {
		return errors.Fmt("tags: %w", err)
	}
	if wu.Properties != nil {
		if err := pbutil.ValidateWorkUnitProperties(wu.Properties); err != nil {
			return errors.Fmt("properties: %w", err)
		}
	}
	if wu.ExtendedProperties != nil {
		if err := pbutil.ValidateInvocationExtendedProperties(wu.ExtendedProperties); err != nil {
			return errors.Fmt("extended_properties: %w", err)
		}
	}
	if wu.Instructions != nil {
		if err := pbutil.ValidateInstructions(wu.Instructions); err != nil {
			return errors.Fmt("instructions: %w", err)
		}
	}
	return nil
}

// validateDeadline validates a deadline for a root invocation or work unit.
// Returns a non-nil error if deadline is invalid.
func validateDeadline(deadline *timestamppb.Timestamp, createTime time.Time) error {
	internal.AssertUTC(createTime)
	if err := deadline.CheckValid(); err != nil {
		return err
	}

	d := deadline.AsTime()
	if d.Sub(createTime) < 10*time.Second {
		return errors.New("must be at least 10 seconds in the future")
	}
	if d.Sub(createTime) > maxDeadlineDuration {
		return errors.Fmt("must be before %dh in the future", int(maxDeadlineDuration.Hours()))
	}
	return nil
}
