// Copyright 2024 The LUCI Authors.
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

// Package gitiles contains logic for interacting with gitiles.
package gitiles

import (
	"context"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
)

// FakeClient is a fake implementation of gitiles.GitilesClient for testing.
type FakeClient struct {
	MakeCommit func(i int32) *git.Commit
}

// UseFakeClient installs a fake gitiles client into the context,
func UseFakeClient(ctx context.Context, makeCommit func(i int32) *git.Commit) context.Context {
	fake := FakeClient{MakeCommit: makeCommit}
	return context.WithValue(ctx, &testingGitilesClientKey, fake)
}

func (f *FakeClient) Log(ctx context.Context, in *gitilespb.LogRequest, opts ...grpc.CallOption) (*gitilespb.LogResponse, error) {
	res := make([]*git.Commit, 0, in.PageSize)
	for i := in.PageSize; i > 0; i-- {
		res = append(res, f.MakeCommit(i))
	}
	return &gitilespb.LogResponse{
		Log:           res,
		NextPageToken: "",
	}, nil
}

func (f *FakeClient) Refs(ctx context.Context, in *gitilespb.RefsRequest, opts ...grpc.CallOption) (*gitilespb.RefsResponse, error) {
	panic("not implemented")
}

func (f *FakeClient) Archive(ctx context.Context, in *gitilespb.ArchiveRequest, opts ...grpc.CallOption) (*gitilespb.ArchiveResponse, error) {
	panic("not implemented")
}

func (f *FakeClient) DownloadFile(ctx context.Context, in *gitilespb.DownloadFileRequest, opts ...grpc.CallOption) (*gitilespb.DownloadFileResponse, error) {
	panic("not implemented")
}

func (f *FakeClient) DownloadDiff(ctx context.Context, in *gitilespb.DownloadDiffRequest, opts ...grpc.CallOption) (*gitilespb.DownloadDiffResponse, error) {
	panic("not implemented")
}

func (f *FakeClient) Projects(ctx context.Context, in *gitilespb.ProjectsRequest, opts ...grpc.CallOption) (*gitilespb.ProjectsResponse, error) {
	panic("not implemented")
}

func (f *FakeClient) ListFiles(ctx context.Context, in *gitilespb.ListFilesRequest, opts ...grpc.CallOption) (*gitilespb.ListFilesResponse, error) {
	panic("not implemented")
}
