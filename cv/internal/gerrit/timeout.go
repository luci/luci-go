// Copyright 2021 The LUCI Authors.
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

package gerrit

import (
	"context"
	"time"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

// TimeLimitedFactory limits duration per each kind of Gerrit RPC.
func TimeLimitedFactory(f ClientFactory) ClientFactory {
	return func(ctx context.Context, gerritHost, luciProject string) (Client, error) {
		c, err := f(ctx, gerritHost, luciProject)
		if err != nil {
			return nil, err
		}
		return timeLimitedClient{
			actual: c,
		}, nil
	}
}

type timeLimitedClient struct {
	actual Client
}

func (i timeLimitedClient) ListChanges(ctx context.Context, in *gerritpb.ListChangesRequest, opts ...grpc.CallOption) (*gerritpb.ListChangesResponse, error) {
	ctx, cancel := clock.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return i.actual.ListChanges(ctx, in, opts...)
}

func (i timeLimitedClient) GetChange(ctx context.Context, in *gerritpb.GetChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
	ctx, cancel := clock.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return i.actual.GetChange(ctx, in, opts...)
}

func (i timeLimitedClient) GetRelatedChanges(ctx context.Context, in *gerritpb.GetRelatedChangesRequest, opts ...grpc.CallOption) (*gerritpb.GetRelatedChangesResponse, error) {
	ctx, cancel := clock.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return i.actual.GetRelatedChanges(ctx, in, opts...)
}

func (i timeLimitedClient) ListFiles(ctx context.Context, in *gerritpb.ListFilesRequest, opts ...grpc.CallOption) (*gerritpb.ListFilesResponse, error) {
	ctx, cancel := clock.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	return i.actual.ListFiles(ctx, in, opts...)
}

func (i timeLimitedClient) SetReview(ctx context.Context, in *gerritpb.SetReviewRequest, opts ...grpc.CallOption) (*gerritpb.ReviewResult, error) {
	ctx, cancel := clock.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
	return i.actual.SetReview(ctx, in, opts...)
}

func (i timeLimitedClient) SubmitRevision(ctx context.Context, in *gerritpb.SubmitRevisionRequest, opts ...grpc.CallOption) (*gerritpb.SubmitInfo, error) {
	// 2 minute is based on single-CL submission.
	// If CV starts using SubmitRevision for 2+ CLs, this may need to be
	// revisited.
	ctx, cancel := clock.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return i.actual.SubmitRevision(ctx, in, opts...)
}
