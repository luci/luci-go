// Copyright 2026 The LUCI Authors.
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

package base

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "go.chromium.org/luci/buildbucket/proto"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type mockGerritClient struct {
	gerritpb.GerritClient
	getChange func(ctx context.Context, in *gerritpb.GetChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error)
}

func (m *mockGerritClient) GetChange(ctx context.Context, in *gerritpb.GetChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
	if m.getChange != nil {
		return m.getChange(ctx, in, opts...)
	}
	return nil, status.Errorf(codes.Unimplemented, "not implemented")
}

func TestParseGerritCL(t *testing.T) {
	t.Parallel()

	ftt.Run(`ParseGerritCL`, t, func(t *ftt.Test) {
		t.Run(`Full Gerrit URL with project and patchset`, func(t *ftt.Test) {
			gc, err := ParseGerritCL("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/8565529/10")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   8565529,
				Patchset: 10,
			}))
		})

		t.Run(`Full Gerrit URL without patchset`, func(t *ftt.Test) {
			gc, err := ParseGerritCL("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/8565529")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   8565529,
				Patchset: 0,
			}))
		})

		t.Run(`Full Gerrit URL with trailing file path`, func(t *ftt.Test) {
			gc, err := ParseGerritCL("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/8565529/10/client/cmd/luci/main.go")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   8565529,
				Patchset: 10,
			}))
		})

		t.Run(`Gerrit URL with legacy hash routing`, func(t *ftt.Test) {
			gc, err := ParseGerritCL("https://chromium-review.googlesource.com/#/c/infra/luci/luci-go/+/8565529/3")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   8565529,
				Patchset: 3,
			}))
		})

		t.Run(`Gerrit URL with legacy hash routing and query params`, func(t *ftt.Test) {
			gc, err := ParseGerritCL("https://chromium-review.googlesource.com/#/c/infra/luci/luci-go/+/8565529?tab=checks")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   8565529,
				Patchset: 0,
			}))
		})

		t.Run(`Git corp google URL normalized to googlesource`, func(t *ftt.Test) {
			gc, err := ParseGerritCL("https://chromium-review.git.corp.google.com/c/infra/luci/luci-go/+/8565529")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   8565529,
				Patchset: 0,
			}))

			gc2, err := ParseGerritCL("https://googleplex-android-review.git.corp.google.com/c/platform/frameworks/base/+/12345/4")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc2, should.Match(&pb.GerritChange{
				Host:     "googleplex-android-review.googlesource.com",
				Project:  "platform/frameworks/base",
				Change:   12345,
				Patchset: 4,
			}))
		})

		t.Run(`Patchset diff range URL`, func(t *ftt.Test) {
			gc, err := ParseGerritCL("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/8565529/1..3")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   8565529,
				Patchset: 3,
			}))

			gc2, err := ParseGerritCL("https://crrev.com/c/8565529/2..5")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc2, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   8565529,
				Patchset: 5,
			}))
		})

		t.Run(`Android ag/ and aosp/ shorthands`, func(t *ftt.Test) {
			gc, err := ParseGerritCL("ag/12345")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc, should.Match(&pb.GerritChange{
				Host:     "googleplex-android-review.googlesource.com",
				Change:   12345,
				Patchset: 0,
			}))

			gc2, err := ParseGerritCL("https://ag/12345/6")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc2, should.Match(&pb.GerritChange{
				Host:     "googleplex-android-review.googlesource.com",
				Change:   12345,
				Patchset: 6,
			}))

			gc3, err := ParseGerritCL("ag/12345/1..4")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc3, should.Match(&pb.GerritChange{
				Host:     "googleplex-android-review.googlesource.com",
				Change:   12345,
				Patchset: 4,
			}))

			gc4, err := ParseGerritCL("aosp/67890")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc4, should.Match(&pb.GerritChange{
				Host:     "android-review.googlesource.com",
				Change:   67890,
				Patchset: 0,
			}))
		})

		t.Run(`Gerrit URL without project`, func(t *ftt.Test) {
			gc, err := ParseGerritCL("https://chromium-review.googlesource.com/c/8565529/2")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   8565529,
				Patchset: 2,
			}))
		})

		t.Run(`Chrome Internal review URL`, func(t *ftt.Test) {
			gc, err := ParseGerritCL("https://chrome-internal-review.googlesource.com/c/chrome/infra/+/12345/4")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc, should.Match(&pb.GerritChange{
				Host:     "chrome-internal-review.googlesource.com",
				Project:  "chrome/infra",
				Change:   12345,
				Patchset: 4,
			}))
		})

		t.Run(`Other googlesource host`, func(t *ftt.Test) {
			gc, err := ParseGerritCL("https://aomedia-review.googlesource.com/c/aom/+/9876/1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc, should.Match(&pb.GerritChange{
				Host:     "aomedia-review.googlesource.com",
				Project:  "aom",
				Change:   9876,
				Patchset: 1,
			}))
		})

		t.Run(`crrev.com shorthand`, func(t *ftt.Test) {
			gc, err := ParseGerritCL("https://crrev.com/c/8565529/3")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   8565529,
				Patchset: 3,
			}))
		})

		t.Run(`crrev.com internal shorthand without scheme`, func(t *ftt.Test) {
			gc, err := ParseGerritCL("crrev.com/i/12345")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc, should.Match(&pb.GerritChange{
				Host:     "chrome-internal-review.googlesource.com",
				Change:   12345,
				Patchset: 0,
			}))
		})

		t.Run(`c/123 shorthand`, func(t *ftt.Test) {
			gc, err := ParseGerritCL("c/8565529/5")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   8565529,
				Patchset: 5,
			}))
		})

		t.Run(`i/123 shorthand`, func(t *ftt.Test) {
			gc, err := ParseGerritCL("i/12345")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc, should.Match(&pb.GerritChange{
				Host:     "chrome-internal-review.googlesource.com",
				Change:   12345,
				Patchset: 0,
			}))
		})

		t.Run(`Bare change ID`, func(t *ftt.Test) {
			gc, err := ParseGerritCL("8565529")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gc, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   8565529,
				Patchset: 0,
			}))
		})

		t.Run(`Invalid targets`, func(t *ftt.Test) {
			_, err := ParseGerritCL("")
			assert.Loosely(t, err, should.NotBeNil)

			_, err = ParseGerritCL("not-a-cl")
			assert.Loosely(t, err, should.NotBeNil)

			_, err = ParseGerritCL("https://example.com/foo")
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

func TestIsGerritURL(t *testing.T) {
	t.Parallel()

	ftt.Run(`IsGerritURL`, t, func(t *ftt.Test) {
		assert.Loosely(t, IsGerritURL("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/8565529"), should.BeTrue)
		assert.Loosely(t, IsGerritURL("https://chromium-review.git.corp.google.com/c/infra/luci/luci-go/+/8565529"), should.BeTrue)
		assert.Loosely(t, IsGerritURL("https://chromium-review.googlesource.com/#/c/infra/luci/luci-go/+/8565529?tab=checks"), should.BeTrue)
		assert.Loosely(t, IsGerritURL("https://crrev.com/c/8565529"), should.BeTrue)
		assert.Loosely(t, IsGerritURL("crrev.com/i/12345"), should.BeTrue)
		assert.Loosely(t, IsGerritURL("c/8565529"), should.BeTrue)
		assert.Loosely(t, IsGerritURL("i/12345"), should.BeTrue)
		assert.Loosely(t, IsGerritURL("ag/12345"), should.BeTrue)
		assert.Loosely(t, IsGerritURL("https://ag/12345/3"), should.BeTrue)
		assert.Loosely(t, IsGerritURL("aosp/67890"), should.BeTrue)

		// Bare numbers must NOT be detected as Gerrit URLs (they may be Build IDs in `luci ids`)
		assert.Loosely(t, IsGerritURL("8565529"), should.BeFalse)
		assert.Loosely(t, IsGerritURL("8738491827364512345"), should.BeFalse)
		assert.Loosely(t, IsGerritURL("https://ci.chromium.org/b/12345"), should.BeFalse)
		assert.Loosely(t, IsGerritURL(""), should.BeFalse)
	})
}

func TestResolveGerritPatchset(t *testing.T) {
	t.Parallel()

	ftt.Run(`ResolveGerritPatchset`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`Already has patchset - returns immediately`, func(t *ftt.Test) {
			change := &pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   8565529,
				Patchset: 3,
			}
			resolved, err := ResolveGerritPatchset(ctx, nil, change)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resolved.Patchset, should.Equal(3))
		})

		t.Run(`Missing patchset with mock client`, func(t *ftt.Test) {
			mock := &mockGerritClient{
				getChange: func(ctx context.Context, in *gerritpb.GetChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
					assert.Loosely(t, in.Number, should.Equal(8565529))
					return &gerritpb.ChangeInfo{
						Number:          8565529,
						Project:         "infra/luci/luci-go",
						CurrentRevision: "commit_sha_rev_5",
						Revisions: map[string]*gerritpb.RevisionInfo{
							"commit_sha_rev_4": {Number: 4},
							"commit_sha_rev_5": {Number: 5},
						},
					}, nil
				},
			}

			change := &pb.GerritChange{
				Host:   "chromium-review.googlesource.com",
				Change: 8565529,
			}
			resolved, err := ResolveGerritPatchset(ctx, mock, change)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resolved.Patchset, should.Equal(5))
			assert.Loosely(t, resolved.Project, should.Equal("infra/luci/luci-go"))
		})

		t.Run(`Missing patchset with fallback to max revision`, func(t *ftt.Test) {
			mock := &mockGerritClient{
				getChange: func(ctx context.Context, in *gerritpb.GetChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
					return &gerritpb.ChangeInfo{
						Number:          8565529,
						CurrentRevision: "unknown_sha",
						Revisions: map[string]*gerritpb.RevisionInfo{
							"commit_sha_rev_1": {Number: 1},
							"commit_sha_rev_7": {Number: 7},
						},
					}, nil
				},
			}

			change := &pb.GerritChange{
				Host:   "chromium-review.googlesource.com",
				Change: 8565529,
			}
			resolved, err := ResolveGerritPatchset(ctx, mock, change)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resolved.Patchset, should.Equal(7))
		})

		t.Run(`Passes project to GetChange if already known`, func(t *ftt.Test) {
			mock := &mockGerritClient{
				getChange: func(ctx context.Context, in *gerritpb.GetChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
					assert.Loosely(t, in.Number, should.Equal(8565529))
					assert.Loosely(t, in.Project, should.Equal("infra/luci/luci-go"))
					return &gerritpb.ChangeInfo{
						Number:          8565529,
						Project:         "infra/luci/luci-go",
						CurrentRevision: "commit_sha_rev_2",
						Revisions: map[string]*gerritpb.RevisionInfo{
							"commit_sha_rev_2": {Number: 2},
						},
					}, nil
				},
			}

			change := &pb.GerritChange{
				Host:    "chromium-review.googlesource.com",
				Project: "infra/luci/luci-go",
				Change:  8565529,
			}
			resolved, err := ResolveGerritPatchset(ctx, mock, change)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resolved.Patchset, should.Equal(2))
		})

		t.Run(`Nil client returns error`, func(t *ftt.Test) {
			change := &pb.GerritChange{
				Host:   "chromium-review.googlesource.com",
				Change: 8565529,
			}
			_, err := ResolveGerritPatchset(ctx, nil, change)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("patchset is required"))
		})

		t.Run(`RPC failure propagates`, func(t *ftt.Test) {
			mock := &mockGerritClient{
				getChange: func(ctx context.Context, in *gerritpb.GetChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
					return nil, status.Errorf(codes.NotFound, "change not found")
				},
			}

			change := &pb.GerritChange{
				Host:   "chromium-review.googlesource.com",
				Change: 8565529,
			}
			_, err := ResolveGerritPatchset(ctx, mock, change)
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}
