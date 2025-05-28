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

package buildbucket

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
)

// FakeClient is a fake implementation of bbpb.BuildsClient for testing.
type FakeClient struct {
	Builds map[int64]*bbpb.Build
}

// UseFakeClient installs a fake buildbucket client into the context,
// with the given build data.
func UseFakeClient(ctx context.Context, builds map[int64]*bbpb.Build) context.Context {
	return useBuildsClientForTesting(ctx, &FakeClient{Builds: builds})
}

// GetBuild provides an implementation of the GetBuild RPC using an
// in-memory list of builds.
func (f *FakeClient) GetBuild(ctx context.Context, in *bbpb.GetBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error) {
	if r, ok := f.Builds[in.Id]; ok {
		// Incorrect field masks by callers are a common source of errors.
		// Make sure the fake implementation applies the caller-specified
		// mask to allow detection of incorrect masks in tests.
		buildCopy := proto.Clone(r).(*bbpb.Build)
		if in.Mask != nil {
			mask, err := mask.FromFieldMask(in.Mask.Fields, &bbpb.Build{}, mask.AdvancedSemantics())
			if err != nil {
				return nil, errors.Fmt("invalid field mask: %w", err)
			}
			if err := mask.Trim(buildCopy); err != nil {
				return nil, errors.Fmt("apply field mask: %w", err)
			}
		}
		return buildCopy, nil
	}
	return nil, errors.New("not found")
}
