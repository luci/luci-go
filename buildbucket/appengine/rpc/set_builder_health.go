// Copyright 2023 The LUCI Authors.
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

package rpc

import (
	"context"
	"strings"

	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/protowalk"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
)

type SetBuilderHealthChecker struct{}

var _ protowalk.FieldProcessor = (*SetBuilderHealthChecker)(nil)

func (SetBuilderHealthChecker) Process(_ protowalk.DataMap, field protoreflect.FieldDescriptor, msg protoreflect.Message) (data protowalk.ResultData, applied bool) {
	return protowalk.ResultData{Message: "required", IsErr: true}, true
}

func (SetBuilderHealthChecker) ShouldProcess(field protoreflect.FieldDescriptor) protowalk.ProcessAttr {
	if fo := field.Options().(*descriptorpb.FieldOptions); fo != nil {
		required := proto.GetExtension(fo, pb.E_RequiredByRpc).([]string)
		for _, r := range required {
			if r == "SetBuilderHealth" {
				return protowalk.ProcessIfUnset
			}
		}
	}
	return protowalk.ProcessNever
}

// createErrorResponse creates an errored response entry based on the error and code.
func createErrorResponse(err error, code codes.Code) *pb.SetBuilderHealthResponse_Response {
	return &pb.SetBuilderHealthResponse_Response{
		Response: &pb.SetBuilderHealthResponse_Response_Error{
			Error: &status.Status{
				Code:    int32(code),
				Message: err.Error(),
			},
		},
	}
}

func annotateErrorWithBuilder(err error, builder *pb.BuilderID) error {
	return errors.WrapIf(err, "Builder: %s/%s/%s", builder.Project, builder.Bucket, builder.Builder)
}

var sbhrWalker = protowalk.NewWalker[*pb.SetBuilderHealthRequest](
	protowalk.RequiredProcessor{},
	SetBuilderHealthChecker{},
)

// validateRequest validates if the given request is valid or not. It also modifies
// resp to add any new errors that arise from the request validation.
func validateRequest(ctx context.Context, req *pb.SetBuilderHealthRequest, errs map[int]error, resp []*pb.SetBuilderHealthResponse_Response) error {
	if procRes := sbhrWalker.Execute(req); !procRes.Empty() {
		if resStrs := procRes.Strings(); len(resStrs) > 0 {
			logging.Infof(ctx, strings.Join(resStrs, ". "))
		}
		if err := procRes.Err(); err != nil {
			return err
		}
	}
	seen := stringset.New(len(req.Health))
	for i, msg := range req.Health {
		fullBldrID := strings.Join([]string{msg.Id.Project, msg.Id.Bucket, msg.Id.Builder}, "/")
		if seen.Has(fullBldrID) {
			return errors.Fmt("The following builder has multiple entries: %s", fullBldrID)
		}
		seen.Add(fullBldrID)
		if errs[i] == nil && (msg.Health.GetHealthScore() < 0 || msg.Health.GetHealthScore() > 10) {
			err := annotateErrorWithBuilder(errors.New("HealthScore should be between 0 and 10"), msg.Id)
			errs[i] = err
			resp[i] = createErrorResponse(err, codes.InvalidArgument)
		}
	}
	return nil
}

// updateBuilderEntityWithHealth performs a read operation on the builder datastore model, updates
// the metadata, then saves the builder back to datastore.
func updateBuilderEntityWithHealth(ctx context.Context, bldr *pb.SetBuilderHealthRequest_BuilderHealth) error {
	bktKey := model.BucketKey(ctx, bldr.Id.Project, bldr.Id.Bucket)
	builder := &model.Builder{
		ID:     bldr.Id.Builder,
		Parent: bktKey,
	}
	txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		err := datastore.Get(ctx, builder)
		if err != nil {
			if _, isAppStatusErr := appstatus.Get(err); isAppStatusErr {
				return err
			}
			return appstatus.Errorf(codes.Internal, "failed to get builder %s: %s", bldr.Id.Builder, err)
		}
		// If the reporter did not provide data and doc links, use the default ones from the builder config.
		if bldr.Health.DataLinks == nil {
			bldr.Health.DataLinks = builder.Config.GetBuilderHealthMetricsLinks().GetDataLinks()
		}
		if bldr.Health.DocLinks == nil {
			bldr.Health.DocLinks = builder.Config.GetBuilderHealthMetricsLinks().GetDocLinks()
		}
		if bldr.Health.ContactTeamEmail == "" {
			bldr.Health.ContactTeamEmail = builder.Config.GetContactTeamEmail()
		}
		bldr.Health.ReportedTime = timestamppb.Now()
		bldr.Health.Reporter = auth.CurrentIdentity(ctx).Value()
		if builder.Metadata != nil {
			builder.Metadata.Health = bldr.Health
		} else {
			builder.Metadata = &pb.BuilderMetadata{
				Health: bldr.Health,
			}
		}
		return datastore.Put(ctx, builder)
	}, nil)
	return txErr
}

// SetBuilderHealth implements pb.Builds.SetBuilderHealth.
func (*Builders) SetBuilderHealth(ctx context.Context, req *pb.SetBuilderHealthRequest) (*pb.SetBuilderHealthResponse, error) {
	// Create and populate resp with empty protos
	resp := &pb.SetBuilderHealthResponse{}
	if len(req.GetHealth()) == 0 {
		return resp, nil
	}
	resp.Responses = make([]*pb.SetBuilderHealthResponse_Response, len(req.Health))
	for i := 0; i < len(req.Health); i++ {
		resp.Responses[i] = &pb.SetBuilderHealthResponse_Response{
			Response: &pb.SetBuilderHealthResponse_Response_Result{
				Result: &emptypb.Empty{},
			},
		}
	}
	// Only want to store health for builders that the requestor has permission
	// to store health for.
	errs := make(map[int]error, len(req.Health))
	for i, msg := range req.Health {
		err := perm.HasInBuilder(ctx, bbperms.BuildersSetHealth, msg.Id)
		if err != nil {
			err := annotateErrorWithBuilder(err, msg.Id)
			errs[i] = err
			resp.Responses[i] = createErrorResponse(err, codes.PermissionDenied)
		}
	}
	// Returning early if there are no builders that the user is allowed to update.
	if len(errs) == len(req.Health) {
		return resp, nil
	}
	if err := validateRequest(ctx, req, errs, resp.Responses); err != nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "%s", err.Error())
	}
	// Finally update builder health for builders that did not have a permission
	// or validation error.
	for i, msg := range req.Health {
		if errs[i] == nil {
			if err := updateBuilderEntityWithHealth(ctx, msg); err != nil {
				resp.Responses[i] = createErrorResponse(err, codes.Internal)
			}
		}
	}
	return resp, nil
}
