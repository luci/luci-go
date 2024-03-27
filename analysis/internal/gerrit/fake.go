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

package gerrit

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

// fakeClientFactory produces FakeClients.
type fakeClientFactory struct {
	clsByHost map[string][]*gerritpb.ChangeInfo
}

// FakeClient is a fake implementation of gerritpb.GerritClient for testing.
type FakeClient struct {
	Changelists []*gerritpb.ChangeInfo
}

// UseFakeClient installs a fake gerrit client into the context,
// with the given gerrit change data.
func UseFakeClient(ctx context.Context, clsByHost map[string][]*gerritpb.ChangeInfo) context.Context {
	clientFactory := &fakeClientFactory{clsByHost: clsByHost}
	return context.WithValue(ctx, &testingGerritClientKey, clientFactory)
}

func (f *fakeClientFactory) WithHost(host string) gerritpb.GerritClient {
	cls, ok := f.clsByHost[host]
	if !ok {
		panic(fmt.Sprintf("Caller asked for invalid gerrit host %s", host))
	}
	return &FakeClient{Changelists: cls}
}

func (f *FakeClient) GetChange(ctx context.Context, in *gerritpb.GetChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
	var result *gerritpb.ChangeInfo
	for _, cl := range f.Changelists {
		if cl.Number != in.Number {
			continue
		}
		if in.Project != "" && cl.Project != in.Project {
			return nil, status.Error(codes.InvalidArgument, "mismatch between CL project in gerrit and supplied project")
		}
		result = cl
	}
	if result == nil {
		return nil, status.Error(codes.NotFound, "CL not found")
	}
	// Copy the result to avoid the caller getting an alias to our internal
	// copy.
	result = proto.Clone(result).(*gerritpb.ChangeInfo)

	detailedAccounts := false
	for _, opt := range in.Options {
		if opt == gerritpb.QueryOption_DETAILED_ACCOUNTS {
			detailedAccounts = true
		}
	}
	if result.Owner != nil && !detailedAccounts {
		result.Owner.Email = ""
		result.Owner.Name = ""
		result.Owner.SecondaryEmails = nil
		result.Owner.Username = ""
	}
	return result, nil
}

func (f *FakeClient) ListProjects(ctx context.Context, in *gerritpb.ListProjectsRequest, opts ...grpc.CallOption) (*gerritpb.ListProjectsResponse, error) {
	panic("not implemented")
}

func (f *FakeClient) GetRefInfo(ctx context.Context, in *gerritpb.RefInfoRequest, opts ...grpc.CallOption) (*gerritpb.RefInfo, error) {
	panic("not implemented")
}

func (f *FakeClient) ListFileOwners(ctx context.Context, in *gerritpb.ListFileOwnersRequest, opts ...grpc.CallOption) (*gerritpb.ListOwnersResponse, error) {
	panic("not implemented")
}

func (f *FakeClient) ListAccountEmails(ctx context.Context, in *gerritpb.ListAccountEmailsRequest, opts ...grpc.CallOption) (*gerritpb.ListAccountEmailsResponse, error) {
	panic("not implemented")
}

func (f *FakeClient) ListChanges(ctx context.Context, in *gerritpb.ListChangesRequest, opts ...grpc.CallOption) (*gerritpb.ListChangesResponse, error) {
	panic("not implemented")
}

func (f *FakeClient) GetMergeable(ctx context.Context, in *gerritpb.GetMergeableRequest, opts ...grpc.CallOption) (*gerritpb.MergeableInfo, error) {
	panic("not implemented")
}

func (f *FakeClient) ListFiles(ctx context.Context, in *gerritpb.ListFilesRequest, opts ...grpc.CallOption) (*gerritpb.ListFilesResponse, error) {
	panic("not implemented")
}

func (f *FakeClient) GetRelatedChanges(ctx context.Context, in *gerritpb.GetRelatedChangesRequest, opts ...grpc.CallOption) (*gerritpb.GetRelatedChangesResponse, error) {
	panic("not implemented")
}

func (f *FakeClient) GetPureRevert(ctx context.Context, in *gerritpb.GetPureRevertRequest, opts ...grpc.CallOption) (*gerritpb.PureRevertInfo, error) {
	panic("not implemented")
}

func (f *FakeClient) GetMetaDiff(ctx context.Context, in *gerritpb.GetMetaDiffRequest, opts ...grpc.CallOption) (*gerritpb.MetaDiff, error) {
	panic("not implemented")
}

func (f *FakeClient) CreateChange(ctx context.Context, in *gerritpb.CreateChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
	panic("not implemented")
}

func (f *FakeClient) ChangeEditFileContent(ctx context.Context, in *gerritpb.ChangeEditFileContentRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	panic("not implemented")
}

func (f *FakeClient) DeleteEditFileContent(ctx context.Context, in *gerritpb.DeleteEditFileContentRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	panic("not implemented")
}

func (f *FakeClient) ChangeEditPublish(ctx context.Context, in *gerritpb.ChangeEditPublishRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	panic("not implemented")
}

func (f *FakeClient) AddReviewer(ctx context.Context, in *gerritpb.AddReviewerRequest, opts ...grpc.CallOption) (*gerritpb.AddReviewerResult, error) {
	panic("not implemented")
}

func (f *FakeClient) DeleteReviewer(ctx context.Context, in *gerritpb.DeleteReviewerRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	panic("not implemented")
}

func (f *FakeClient) SetReview(ctx context.Context, in *gerritpb.SetReviewRequest, opts ...grpc.CallOption) (*gerritpb.ReviewResult, error) {
	panic("not implemented")
}

func (f *FakeClient) AddToAttentionSet(ctx context.Context, in *gerritpb.AttentionSetRequest, opts ...grpc.CallOption) (*gerritpb.AccountInfo, error) {
	panic("not implemented")
}

func (f *FakeClient) SubmitChange(ctx context.Context, in *gerritpb.SubmitChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
	panic("not implemented")
}

func (f *FakeClient) RevertChange(ctx context.Context, in *gerritpb.RevertChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
	panic("not implemented")
}

func (f *FakeClient) AbandonChange(ctx context.Context, in *gerritpb.AbandonChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
	panic("not implemented")
}

func (f *FakeClient) SubmitRevision(ctx context.Context, in *gerritpb.SubmitRevisionRequest, opts ...grpc.CallOption) (*gerritpb.SubmitInfo, error) {
	panic("not implemented")
}
