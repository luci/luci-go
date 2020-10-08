// Copyright 2020 The LUCI Authors.
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

package gitiles

import (
	context "context"
	"fmt"

	"go.chromium.org/luci/common/errors"
	git "go.chromium.org/luci/common/proto/git"
	grpc "google.golang.org/grpc"
)

const defaultPageSize = 100

// key: ref name, value: list of commits
type fakeRepository struct {
	refs    map[string]string
	commits map[string]*git.Commit
}

// Fake allows testing of Gitiles API without using actual Gitiles
// server. User can set data using SetRepository method.
type Fake struct {
	// key: repository name, value fakeRepository
	m        map[string]fakeRepository
	callLogs []interface{}
}

// Log retrieves commit log. Merge commits are supported, but it implements
// simple logic and likely won't return results in the same order as Gitiles.
func (f *Fake) Log(ctx context.Context, in *LogRequest, opts ...grpc.CallOption) (*LogResponse, error) {
	f.addCallLog(in)
	repository, ok := f.m[in.GetProject()]
	if !ok {
		return nil, errors.New("Repository not found")
	}
	committish := in.GetCommittish()
	// Check if committish is a ref
	if commitID, ok := repository.refs[committish]; ok {
		committish = commitID
	}
	commit, ok := repository.commits[committish]
	if !ok {
		return nil, fmt.Errorf("Commit %s not found", committish)
	}
	size := int(in.GetPageSize())
	if size == 0 {
		size = defaultPageSize
	}

	resp := &LogResponse{
		Log: []*git.Commit{},
	}
	startAdding := in.GetPageToken() == ""
	q := []*git.Commit{commit}
	visited := map[string]struct{}{}
	for size > len(resp.Log) && len(q) > 0 {
		commit = q[0]
		q = q[1:]
		if _, ok := visited[commit.GetId()]; ok {
			continue
		}
		visited[commit.GetId()] = struct{}{}

		if startAdding {
			resp.Log = append(resp.Log, commit)
		} else if commit.GetId() == in.GetPageToken() {
			startAdding = true
		}

		for _, commitID := range commit.GetParents() {
			if in.GetExcludeAncestorsOf() == commitID {
				break
			}
			c, ok := repository.commits[commitID]
			if !ok {
				panic(fmt.Sprintf(
					"Broken git chain, commit %s has parent %s which doesn't exist",
					commit.GetId(), commitID))
			}
			q = append(q, c)
		}
	}
	if len(resp.Log) == size {
		resp.NextPageToken = commit.GetId()
	}
	return resp, nil
}

// Refs retrieves repo refs.
func (f *Fake) Refs(ctx context.Context, in *RefsRequest, opts ...grpc.CallOption) (*RefsResponse, error) {
	f.addCallLog(in)
	p, ok := f.m[in.GetProject()]
	if !ok {
		return nil, errors.New("Repository not found")
	}
	resp := &RefsResponse{
		Revisions: p.refs,
	}
	return resp, nil
}

// Archive retrieves archived contents of the project. This is not implemented.
//
// An archive is a shallow bundle of the contents of a repository.
//
// DEPRECATED: Use DownloadFile to obtain plain text files.
// TODO(pprabhu): Migrate known users to DownloadFile and delete this RPC.
func (f *Fake) Archive(ctx context.Context, in *ArchiveRequest, opts ...grpc.CallOption) (*ArchiveResponse, error) {
	f.addCallLog(in)
	panic("not implemented")
}

// DownloadFile retrieves a file from the project. This is not implemented.
func (f *Fake) DownloadFile(ctx context.Context, in *DownloadFileRequest, opts ...grpc.CallOption) (*DownloadFileResponse, error) {
	f.addCallLog(in)
	panic("not implemented")
}

// Projects retrieves list of available Gitiles projects
func (f *Fake) Projects(ctx context.Context, in *ProjectsRequest, opts ...grpc.CallOption) (*ProjectsResponse, error) {
	f.addCallLog(in)
	resp := &ProjectsResponse{
		Projects: make([]string, len(f.m)),
	}
	i := 0
	for projectName := range f.m {
		resp.Projects[i] = projectName
		i++
	}
	return resp, nil
}

// SetRepository stores provided references and commits to desired repository.
// If repository is previously set, it will override it.
//
// refs keys are references, keys are revisions.
// Example:
// f.SetRepository(
//   "foo",
//   []string{"refs/heads/master", "rev1"},
//   []*git.Commit{ {Id: "rev1", Parents: []string{"rev0"}}, {Id: "rev0"} }
// )
// Represents following repository:
// name: foo
// references:
// * refs/heads/master points to rev1
// commits:
// rev1 --> rev0 (root commit)
func (f *Fake) SetRepository(repository string, refs map[string]string, commits []*git.Commit) {
	if f.m == nil {
		f.m = map[string]fakeRepository{}
	}
	commitMap := make(map[string]*git.Commit, len(commits))
	for _, commit := range commits {
		if _, ok := commitMap[commit.GetId()]; ok {
			panic(fmt.Sprintf("Duplicated commit with commit hash: %s", commit.GetId()))
		}
		commitMap[commit.GetId()] = commit
	}
	// Sanity check
	for refs, rev := range refs {
		if rev == "" {
			// empty repository
			continue
		}
		if _, ok := commitMap[rev]; !ok {
			panic(fmt.Sprintf("Ref %s points to invalid revision %s", refs, rev))
		}
	}
	f.m[repository] = fakeRepository{
		refs:    refs,
		commits: commitMap,
	}
}

// GetCallLogs returns callLogs.
func (f *Fake) GetCallLogs() []interface{} {
	return f.callLogs
}

func (f *Fake) addCallLog(in interface{}) {
	f.callLogs = append(f.callLogs, in)
}
