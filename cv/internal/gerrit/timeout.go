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
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

// TimeLimitedFactory limits duration per each kind of Gerrit RPC.
func TimeLimitedFactory(f Factory) Factory {
	return timeLimitedFactory{Factory: f}
}

type timeLimitedFactory struct {
	Factory
}

// MakeClient implements Factory.
func (t timeLimitedFactory) MakeClient(ctx context.Context, gerritHost string, luciProject string) (Client, error) {
	c, err := t.Factory.MakeClient(ctx, gerritHost, luciProject)
	if err != nil {
		return nil, err
	}
	return timeLimitedClient{
		actual:      c,
		luciProject: luciProject,
	}, nil
}

type timeLimitedClient struct {
	actual      Client
	luciProject string
}

func (t timeLimitedClient) ListAccountEmails(ctx context.Context, req *gerritpb.ListAccountEmailsRequest, opts ...grpc.CallOption) (*gerritpb.ListAccountEmailsResponse, error) {
	ctx, cancel := clock.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return t.actual.ListAccountEmails(ctx, req, opts...)
}

func (t timeLimitedClient) ListChanges(ctx context.Context, in *gerritpb.ListChangesRequest, opts ...grpc.CallOption) (*gerritpb.ListChangesResponse, error) {
	ctx, cancel := clock.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return t.actual.ListChanges(ctx, in, opts...)
}

func (t timeLimitedClient) GetChange(ctx context.Context, in *gerritpb.GetChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
	ctx, cancel := clock.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	return t.actual.GetChange(ctx, in, opts...)
}

func (t timeLimitedClient) GetRelatedChanges(ctx context.Context, in *gerritpb.GetRelatedChangesRequest, opts ...grpc.CallOption) (*gerritpb.GetRelatedChangesResponse, error) {
	ctx, cancel := clock.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	return t.actual.GetRelatedChanges(ctx, in, opts...)
}

func (t timeLimitedClient) ListFiles(ctx context.Context, in *gerritpb.ListFilesRequest, opts ...grpc.CallOption) (*gerritpb.ListFilesResponse, error) {
	ctx, cancel := clock.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	return t.actual.ListFiles(ctx, in, opts...)
}

func (t timeLimitedClient) SetReview(ctx context.Context, in *gerritpb.SetReviewRequest, opts ...grpc.CallOption) (*gerritpb.ReviewResult, error) {
	ctx, cancel := clock.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	return t.actual.SetReview(ctx, in, opts...)
}

func (t timeLimitedClient) SubmitRevision(ctx context.Context, in *gerritpb.SubmitRevisionRequest, opts ...grpc.CallOption) (si *gerritpb.SubmitInfo, err error) {
	// TODO(b/267966142): Find a less hacky way to set the timeout for CLs
	// that are known to slow to land.
	if t.isKernelRepo(in.GetProject()) {
		startTime := clock.Now(ctx)
		defer func() {
			if err == nil {
				logging.Infof(ctx, "b/267966142: Took %s to submit change %d at revision %s ", clock.Since(ctx, startTime).Truncate(time.Millisecond), in.GetNumber(), in.GetRevisionId())
			}
		}()
	} else {
		// 2 minute is based on single-CL submission.
		// If CV starts using SubmitRevision to submit 2+ CLs in a stack
		// (a.k.a. "Submit including parents" in Gerrit),
		// this may need to be revisited.
		var cancel context.CancelFunc
		ctx, cancel = clock.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
	}

	return t.actual.SubmitRevision(ctx, in, opts...)
}

func (t timeLimitedClient) isKernelRepo(repo string) bool {
	return t.luciProject == "chromeos" && repo == "chromiumos/third_party/kernel"
}
