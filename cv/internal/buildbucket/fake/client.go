// Copyright 2022 The LUCI Authors.
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

package bbfake

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/buildbucket/appengine/model"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	bbutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"

	"go.chromium.org/luci/cv/internal/buildbucket"
)

type clientFactory struct {
	fake *Fake
}

// MakeClient implements buildbucket.ClientFactory.
func (factory clientFactory) MakeClient(ctx context.Context, host, luciProject string) (buildbucket.Client, error) {
	return &Client{
		fa:          factory.fake.ensureApp(host),
		luciProject: luciProject,
	}, nil
}

// Client connects a Buildbucket Fake and scope to a certain LUCI Project +
// Buildbucket host.
type Client struct {
	fa          *fakeApp
	luciProject string
}

// GetBuild implements buildbucket.Client.
func (c *Client) GetBuild(ctx context.Context, in *bbpb.GetBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error) {
	switch {
	case in.GetBuilder() != nil || in.GetBuildNumber() != 0:
		return nil, status.Errorf(codes.Unimplemented, "GetBuild by builder+number is not supported")
	case in.GetId() == 0:
		return nil, status.Errorf(codes.InvalidArgument, "requested build id is 0")
	}

	switch build := c.fa.getBuild(in.GetId()); {
	case build == nil:
		fallthrough
	case !c.canAccessBuild(build):
		projIdentity := identity.Identity(fmt.Sprintf("%s:%s", identity.Project, c.luciProject))
		return nil, status.Errorf(codes.NotFound, "requested resource not found or %q does not have permission to view it", projIdentity)
	default:
		if err := applyMask(build, in.GetMask()); err != nil {
			return nil, err
		}
		return build, nil
	}
}

var supportedPredicates = stringset.NewFromSlice(
	"gerrit_changes",
	"include_experimental",
)

const defaultSearchPageSize = 5

// SearchBuilds implements buildbucket.Client.
//
// Support paging and the following predicates:
//   - gerrit_changes
//   - include_experimental
//
// Use `defaultSearchPageSize` if page size is not specified in the input.
func (c *Client) SearchBuilds(ctx context.Context, in *bbpb.SearchBuildsRequest, opts ...grpc.CallOption) (*bbpb.SearchBuildsResponse, error) {
	if in.GetPredicate() != nil {
		var notSupportedPredicates []string
		in.GetPredicate().ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
			if v.IsValid() && !supportedPredicates.Has(string(fd.Name())) {
				notSupportedPredicates = append(notSupportedPredicates, string(fd.Name()))
			}
			return true
		})
		if len(notSupportedPredicates) > 0 {
			return nil, status.Errorf(codes.InvalidArgument, "predicates [%s] are not supported", strings.Join(notSupportedPredicates, ", "))
		}
	}
	var lastReturnedBuildID int64
	if token := in.GetPageToken(); token != "" {
		var err error
		lastReturnedBuildID, err = strconv.ParseInt(token, 10, 64)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid token %q, expecting a build ID", token)
		}
	}
	var candidates []*bbpb.Build
	c.fa.iterBuildStore(func(build *bbpb.Build) {
		candidates = append(candidates, build)
	})
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Id < candidates[j].Id
	})
	pageSize := in.GetPageSize()
	if pageSize == 0 {
		pageSize = defaultSearchPageSize
	}
	resBuilds := make([]*bbpb.Build, 0, pageSize)
	for _, b := range candidates {
		if c.shouldIncludeBuild(b, in.GetPredicate(), lastReturnedBuildID) {
			if err := applyMask(b, in.GetMask()); err != nil {
				return nil, err
			}
			resBuilds = append(resBuilds, b)
			if len(resBuilds) == int(pageSize) {
				return &bbpb.SearchBuildsResponse{
					Builds:        resBuilds,
					NextPageToken: strconv.FormatInt(b.Id, 10),
				}, nil
			}
		}
	}
	return &bbpb.SearchBuildsResponse{Builds: resBuilds}, nil
}

func (c *Client) shouldIncludeBuild(b *bbpb.Build, pred *bbpb.BuildPredicate, lastReturnedBuildID int64) bool {
	switch {
	case b.GetId() <= lastReturnedBuildID:
		return false
	case !c.canAccessBuild(b):
		return false
	case !pred.GetIncludeExperimental() && b.GetInput().GetExperimental():
		return false
	case len(pred.GetGerritChanges()) > 0:
		gcs := stringset.New(len(b.GetInput().GetGerritChanges()))
		for _, gc := range b.GetInput().GetGerritChanges() {
			gcs.Add(fmt.Sprintf("%s/%s/%d/%d", gc.GetHost(), gc.GetProject(), gc.GetChange(), gc.GetPatchset()))
		}
		for _, gc := range pred.GetGerritChanges() {
			if !gcs.Has(fmt.Sprintf("%s/%s/%d/%d", gc.GetHost(), gc.GetProject(), gc.GetChange(), gc.GetPatchset())) {
				return false
			}
		}
	}
	return true
}

// CancelBuild implements buildbucket.Client.
func (c *Client) CancelBuild(ctx context.Context, in *bbpb.CancelBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error) {
	if in.GetId() == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "requested build id is 0")
	}
	var noAccess bool
	var updatedBuild *bbpb.Build
	if build := c.fa.getBuild(in.GetId()); build == nil {
		noAccess = true
	} else {
		updatedBuild = c.fa.updateBuild(ctx, in.GetId(), func(build *bbpb.Build) {
			switch {
			case !c.canAccessBuild(build):
				noAccess = true
			case bbutil.IsEnded(build.GetStatus()):
				// noop on ended build
			default:
				build.Status = bbpb.Status_CANCELED
				now := timestamppb.New(clock.Now(ctx).UTC())
				if build.GetStartTime() == nil {
					build.StartTime = now
				}
				build.EndTime = now
				build.UpdateTime = now
				build.SummaryMarkdown = in.GetSummaryMarkdown()
			}
		})
	}

	if noAccess {
		projIdentity := identity.Identity(fmt.Sprintf("%s:%s", identity.Project, c.luciProject))
		return nil, status.Errorf(codes.NotFound, "requested resource not found or %q does not have permission to modify it", projIdentity)
	}

	if err := applyMask(updatedBuild, in.GetMask()); err != nil {
		return nil, err
	}
	return updatedBuild, nil
}

var supportedScheduleArguments = stringset.NewFromSlice(
	"request_id",
	"builder",
	"properties",
	"gerrit_changes",
	"tags",
	"experiments",
	"mask",
)

// ScheduleBuild schedules a new build for the provided builder.
//
// The builder should be present in buildbucket fake. It can be added via
// AddBuilder function.
func (c *Client) ScheduleBuild(ctx context.Context, in *bbpb.ScheduleBuildRequest) (*bbpb.Build, error) {
	var notSupportedArguments []string
	in.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if v.IsValid() && !supportedScheduleArguments.Has(string(fd.Name())) {
			notSupportedArguments = append(notSupportedArguments, string(fd.Name()))
		}
		return true
	})
	if len(notSupportedArguments) > 0 {
		return nil, status.Errorf(codes.InvalidArgument, "schedule arguments [%s] are not supported", strings.Join(notSupportedArguments, ", "))
	}

	if build := c.fa.findDupRequest(ctx, in.GetRequestId()); build != nil {
		if err := applyMask(build, in.GetMask()); err != nil {
			return nil, err
		}
		return build, nil
	}

	builderID := in.GetBuilder()
	if builderID == nil {
		return nil, status.Errorf(codes.InvalidArgument, "requested builder is empty")
	}
	builderCfg := c.fa.loadBuilderCfg(builderID)
	if builderCfg == nil {
		return nil, status.Errorf(codes.NotFound, "builder %s not found", bbutil.FormatBuilderID(builderID))
	}
	inputProps, err := mkInputProps(builderCfg, in.GetProperties())
	if err != nil {
		return nil, err
	}
	now := timestamppb.New(clock.Now(ctx))
	build := &bbpb.Build{
		Builder:    builderID,
		Status:     bbpb.Status_SCHEDULED,
		CreateTime: now,
		UpdateTime: now,
		Input: &bbpb.Build_Input{
			Properties:    inputProps,
			GerritChanges: in.GetGerritChanges(),
		},
		Infra: &bbpb.BuildInfra{
			Buildbucket: &bbpb.BuildInfra_Buildbucket{
				RequestedProperties: in.GetProperties(),
				Hostname:            c.fa.hostname,
			},
		},
		Tags: in.GetTags(),
	}

	if len(in.GetExperiments()) > 0 {
		experiments := make(sort.StringSlice, 0, len(in.GetExperiments()))
		for exp, enabled := range in.GetExperiments() {
			if enabled {
				experiments = append(experiments, exp)
			}
		}
		experiments.Sort()
		if len(experiments) > 0 {
			build.Input.Experiments = experiments
		}
	}

	c.fa.insertBuild(ctx, build, in.GetRequestId())
	if err := applyMask(build, in.GetMask()); err != nil {
		return nil, err
	}
	return build, nil
}

func mkInputProps(builderCfg *bbpb.BuilderConfig, requestedProps *structpb.Struct) (*structpb.Struct, error) {
	var ret *structpb.Struct
	if builderProps := builderCfg.GetProperties(); builderProps != "" {
		ret = &structpb.Struct{}
		if err := protojson.Unmarshal([]byte(builderProps), ret); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to unmarshal properties: %s", builderProps)
		}
	}
	if requestedProps != nil {
		if ret == nil {
			return requestedProps, nil
		}
		proto.Merge(ret, requestedProps)
	}
	return ret, nil
}

// Batch implements buildbucket.Client.
//
// Supports:
//   - CancelBuild
//   - GetBuild
//   - ScheduleBuild
func (c *Client) Batch(ctx context.Context, in *bbpb.BatchRequest, opts ...grpc.CallOption) (*bbpb.BatchResponse, error) {
	responses := make([]*bbpb.BatchResponse_Response, len(in.GetRequests()))
	for i, req := range in.GetRequests() {
		res := &bbpb.BatchResponse_Response{}
		switch req.GetRequest().(type) {
		case *bbpb.BatchRequest_Request_CancelBuild:
			if b, err := c.CancelBuild(ctx, req.GetCancelBuild()); err != nil {
				res.Response = &bbpb.BatchResponse_Response_Error{
					Error: status.Convert(err).Proto(),
				}
			} else {
				res.Response = &bbpb.BatchResponse_Response_CancelBuild{
					CancelBuild: b,
				}
			}
		case *bbpb.BatchRequest_Request_GetBuild:
			if b, err := c.GetBuild(ctx, req.GetGetBuild()); err != nil {
				res.Response = &bbpb.BatchResponse_Response_Error{
					Error: status.Convert(err).Proto(),
				}
			} else {
				res.Response = &bbpb.BatchResponse_Response_GetBuild{
					GetBuild: b,
				}
			}
		case *bbpb.BatchRequest_Request_ScheduleBuild:
			if b, err := c.ScheduleBuild(ctx, req.GetScheduleBuild()); err != nil {
				res.Response = &bbpb.BatchResponse_Response_Error{
					Error: status.Convert(err).Proto(),
				}
			} else {
				res.Response = &bbpb.BatchResponse_Response_ScheduleBuild{
					ScheduleBuild: b,
				}
			}
		default:
			return nil, status.Errorf(codes.Unimplemented, "batch request type: %T is not supported", req.GetRequest())
		}
		responses[i] = res
	}
	return &bbpb.BatchResponse{
		Responses: responses,
	}, nil
}

func (c *Client) canAccessBuild(build *bbpb.Build) bool {
	// TODO(yiwzhang): implement proper ACL
	return c.luciProject == build.GetBuilder().GetProject()
}

func applyMask(build *bbpb.Build, bm *bbpb.BuildMask) error {
	mask, err := model.NewBuildMask("", nil, bm)
	if err != nil {
		return status.Errorf(codes.Internal, "error while constructing BuildMask: %s", err)
	}
	if err := mask.Trim(build); err != nil {
		return status.Errorf(codes.Internal, "error while applying field mask: %s", err)
	}
	return nil
}
