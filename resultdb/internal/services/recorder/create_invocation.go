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
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

var (
	permCreateInvocation = realms.RegisterPermission("resultdb.invocations.create")

	// Internal permissions
	permCreateWithReservedID = realms.RegisterPermission("resultdb.invocations.createWithReservedID")
	permExportToBigQuery     = realms.RegisterPermission("resultdb.invocations.exportToBigQuery")
	permSetProducerResource  = realms.RegisterPermission("resultdb.invocations.setProducerResource")
)

// validateInvocationDeadline returns a non-nil error if deadline is invalid.
func validateInvocationDeadline(deadline *tspb.Timestamp, now time.Time) error {
	internal.AssertUTC(now)
	switch deadline, err := ptypes.Timestamp(deadline); {
	case err != nil:
		return err

	case deadline.Sub(now) < 10*time.Second:
		return errors.Reason("must be at least 10 seconds in the future").Err()

	case deadline.Sub(now) > 2*24*time.Hour:
		return errors.Reason("must be before 48h in the future").Err()

	default:
		return nil
	}
}

// validateCreateInvocationRequest returns an error if req is determined to be
// invalid.
func validateCreateInvocationRequest(req *pb.CreateInvocationRequest, now time.Time) error {
	if err := pbutil.ValidateInvocationID(req.InvocationId); err != nil {
		return errors.Annotate(err, "invocation_id").Err()
	}
	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Annotate(err, "request_id").Err()
	}

	inv := req.Invocation
	if inv == nil {
		return nil
	}

	if err := pbutil.ValidateStringPairs(inv.GetTags()); err != nil {
		return errors.Annotate(err, "invocation.tags").Err()
	}

	// TODO(crbug.com/1013316): Do not allow empty realm.
	if inv.GetRealm() != "" {
		if err := realms.ValidateRealmName(inv.Realm, realms.GlobalScope); err != nil {
			return errors.Annotate(err, "invocation.realm").Err()
		}
	}

	if inv.GetDeadline() != nil {
		if err := validateInvocationDeadline(inv.Deadline, now); err != nil {
			return errors.Annotate(err, "invocation: deadline").Err()
		}
	}

	for i, bqExport := range inv.GetBigqueryExports() {
		if err := pbutil.ValidateBigQueryExport(bqExport); err != nil {
			return errors.Annotate(err, "bigquery_export[%d]", i).Err()
		}
	}

	return nil
}

func verifyCreateInvocationPermissions(ctx context.Context, in *pb.CreateInvocationRequest) error {
	inv := in.GetInvocation()
	realm := inv.GetRealm()

	// TODO(crbug.com/1013316): Remove this fallback when realm is required in
	// all invocation creations.
	if realm == "" || realm == "chromium:public" {
		// legacyTrustedCreator indicates the caller is allowed certain things on realm-less invocations
		// without being explicitly granted these permissions, by virtue of belonging to a trusted group.
		switch legacyTrustedCreator, err := auth.IsMember(ctx, "luci-resultdb-trusted-invocation-creators"); {
		case err != nil:
			return err
		case legacyTrustedCreator:
			return nil
		}
		// Set the default realm to avoid checking permissions against the empty realm.
		realm = "chromium:public"
	}

	switch allowed, err := auth.HasPermission(ctx, permCreateInvocation, realm); {
	case err != nil:
		return err
	case !allowed && realm != "chromium:public":
		return appstatus.Errorf(codes.PermissionDenied, `creator does not have permission to create invocations in realm %q`, realm)
	}

	if !strings.HasPrefix(in.InvocationId, "u-") {
		switch allowed, err := auth.HasPermission(ctx, permCreateWithReservedID, realm); {
		case err != nil:
			return err
		case !allowed:
			return appstatus.Errorf(codes.PermissionDenied, `only invocations created by trusted systems may have id not starting with "u-"; please generate "u-{GUID}" or reach out to ResultDB owners`)
		}
	}

	if len(inv.GetBigqueryExports()) > 0 {
		switch allowed, err := auth.HasPermission(ctx, permExportToBigQuery, realm); {
		case err != nil:
			return err
		case !allowed:
			return appstatus.Errorf(codes.PermissionDenied, `creator does not have permission to set bigquery exports in realm %q`, inv.GetRealm())
		}
	}

	if inv.GetProducerResource() != "" {
		switch allowed, err := auth.HasPermission(ctx, permSetProducerResource, realm); {
		case err != nil:
			return err
		case !allowed:
			return appstatus.Errorf(codes.PermissionDenied, `only invocations created by trusted system may have a populated producer_resource field`)
		}
	}

	return nil
}

// CreateInvocation implements pb.RecorderServer.
func (s *recorderServer) CreateInvocation(ctx context.Context, in *pb.CreateInvocationRequest) (*pb.Invocation, error) {
	now := clock.Now(ctx).UTC()

	if err := verifyCreateInvocationPermissions(ctx, in); err != nil {
		return nil, err
	}

	if err := validateCreateInvocationRequest(in, now); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	invs, tokens, err := s.createInvocations(ctx, []*pb.CreateInvocationRequest{in}, in.RequestId, now, invocations.NewIDSet(invocations.ID(in.InvocationId)))
	if err != nil {
		return nil, err
	}
	if len(invs) != 1 || len(tokens) != 1 {
		panic("createInvocations did not return either an error or a valid invocation/token pair")
	}
	md := metadata.MD{}
	md.Set(UpdateTokenMetadataKey, tokens...)
	prpc.SetHeader(ctx, md)
	return invs[0], nil
}

func invocationAlreadyExists(id invocations.ID) error {
	return appstatus.Errorf(codes.AlreadyExists, "%s already exists", id.Name())
}
