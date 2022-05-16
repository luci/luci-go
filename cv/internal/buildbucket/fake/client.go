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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/buildbucket/appengine/model"
	bbpb "go.chromium.org/luci/buildbucket/proto"
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
	panic("not implemented")
}

var supportedPredicates = stringset.NewFromSlice(
	"gerrit_changes",
	"include_experimental",
)

const defaultSearchPageSize = 5

// SearchBuilds implements buildbucket.Client.
//
// Support paging and the following predicates:
//  * gerrit_changes
//  * include_experimental
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
	candidates := make([]*bbpb.Build, 0, len(c.fa.buildStore))
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
			clone := proto.Clone(b).(*bbpb.Build)
			mask, err := model.NewBuildMask("", nil, in.GetMask())
			if err != nil {
				return nil, status.Errorf(codes.Internal, "error while constructing BuildMask: %s", err)
			}
			if err := mask.Trim(clone); err != nil {
				return nil, status.Errorf(codes.Internal, "error while applying field mask: %s", err)
			}
			resBuilds = append(resBuilds, clone)
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
	panic("not implemented")
}

// Batch implements buildbucket.Client.
func (c *Client) Batch(ctx context.Context, in *bbpb.BatchRequest, opts ...grpc.CallOption) (*bbpb.BatchResponse, error) {
	panic("not implemented")
}

func (c *Client) canAccessBuild(build *bbpb.Build) bool {
	// TODO(yiwzhang): implement proper ACL
	return c.luciProject == build.GetBuilder().GetProject()
}
