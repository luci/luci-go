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

package build

import (
	"bytes"
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/client/cmd/luci/base"
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

func TestParseBuilderFilter(t *testing.T) {
	ftt.Run(`parseBuilderFilter`, t, func(t *ftt.Test) {
		t.Run(`Full project/bucket/builder path`, func(t *ftt.Test) {
			bid, err := parseBuilderFilter("chromium/ci/Linux UBSan Tests")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, bid, should.Match(&pb.BuilderID{
				Project: "chromium",
				Bucket:  "ci",
				Builder: "Linux UBSan Tests",
			}))
		})

		t.Run(`Project and bucket only`, func(t *ftt.Test) {
			bid, err := parseBuilderFilter("chromium/ci")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, bid, should.Match(&pb.BuilderID{
				Project: "chromium",
				Bucket:  "ci",
			}))
		})

		t.Run(`Project only`, func(t *ftt.Test) {
			bid, err := parseBuilderFilter("chromium")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, bid, should.Match(&pb.BuilderID{
				Project: "chromium",
			}))
		})

		t.Run(`Invalid builder format`, func(t *ftt.Test) {
			_, err := parseBuilderFilter("a/b/c/d")
			assert.Loosely(t, err, should.NotBeNil)

			_, err = parseBuilderFilter("chromium//linux-rel")
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

func TestParseTagAndStatusFilters(t *testing.T) {
	ftt.Run(`parseTagFilters and parseBuildStatus`, t, func(t *ftt.Test) {
		t.Run(`Valid tags`, func(t *ftt.Test) {
			tags, err := parseTagFilters([]string{"dut-name:chromeos8-row1-rack10-host45", "buildset:patch/gerrit/chromium-review.googlesource.com/123/4"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tags, should.Match([]*pb.StringPair{
				{Key: "dut-name", Value: "chromeos8-row1-rack10-host45"},
				{Key: "buildset", Value: "patch/gerrit/chromium-review.googlesource.com/123/4"},
			}))
		})

		t.Run(`Invalid tag missing colon or empty value`, func(t *ftt.Test) {
			_, err := parseTagFilters([]string{"invalid-tag"})
			assert.Loosely(t, err, should.NotBeNil)

			_, err = parseTagFilters([]string{"key:"})
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run(`Valid statuses and aliases`, func(t *ftt.Test) {
			st, err := parseBuildStatus("failure")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, st, should.Equal(pb.Status_FAILURE))

			st, err = parseBuildStatus("FAILED")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, st, should.Equal(pb.Status_FAILURE))

			st, err = parseBuildStatus("success")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, st, should.Equal(pb.Status_SUCCESS))

			st, err = parseBuildStatus("infra-failure")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, st, should.Equal(pb.Status_INFRA_FAILURE))

			st, err = parseBuildStatus("ended")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, st, should.Equal(pb.Status_ENDED_MASK))
		})

		t.Run(`Invalid status`, func(t *ftt.Test) {
			_, err := parseBuildStatus("unknown_status")
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

func TestBuildSearchPredicate(t *testing.T) {
	ftt.Run(`buildSearchPredicate`, t, func(t *ftt.Test) {
		t.Run(`GerritChanges predicate alone`, func(t *ftt.Test) {
			gc := &pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   8565529,
				Patchset: 10,
			}
			p, err := buildSearchPredicate("", nil, "", gc)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, p.GerritChanges, should.Match([]*pb.GerritChange{gc}))
			assert.Loosely(t, p.Builder, should.BeNil)
			assert.Loosely(t, p.Status, should.Equal(pb.Status_STATUS_UNSPECIFIED))
		})

		t.Run(`Combined with builder, status, and tag`, func(t *ftt.Test) {
			gc := &pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   8565529,
				Patchset: 3,
			}
			p, err := buildSearchPredicate("chromium/try/linux-rel", []string{"os:linux"}, "failure", gc)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, p.GerritChanges, should.Match([]*pb.GerritChange{gc}))
			assert.Loosely(t, p.Builder, should.Match(&pb.BuilderID{
				Project: "chromium",
				Bucket:  "try",
				Builder: "linux-rel",
			}))
			assert.Loosely(t, p.Status, should.Equal(pb.Status_FAILURE))
			assert.Loosely(t, p.Tags, should.Match([]*pb.StringPair{
				{Key: "os", Value: "linux"},
			}))
		})

		t.Run(`All empty returns error`, func(t *ftt.Test) {
			_, err := buildSearchPredicate("", nil, "", nil)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("at least one filter flag"))
		})
	})
}

func TestFormatBuildList(t *testing.T) {
	ftt.Run(`FormatBuildList`, t, func(t *ftt.Test) {
		startTime := time.Date(2026, 9, 8, 7, 0, 0, 0, time.UTC)
		endTime := time.Date(2026, 9, 8, 7, 18, 30, 0, time.UTC)

		t.Run(`Failed build with truncated summary`, func(t *ftt.Test) {
			builds := []*pb.Build{
				{
					Id:     8738491827364512346,
					Number: 12346,
					Builder: &pb.BuilderID{
						Project: "chromium",
						Bucket:  "ci",
						Builder: "Linux UBSan Tests",
					},
					Status:          pb.Status_FAILURE,
					SummaryMarkdown: "1 test failed\nMore details here",
					CreateTime:      timestamppb.New(startTime.Add(-5 * time.Second)),
					StartTime:       timestamppb.New(startTime),
					EndTime:         timestamppb.New(endTime),
				},
			}

			var buf bytes.Buffer
			err := FormatBuildList(&buf, builds)
			assert.Loosely(t, err, should.BeNil)

			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Builds (1):"))
			assert.Loosely(t, out, should.ContainSubstring("Build:         chromium/ci/Linux UBSan Tests/12346"))
			assert.Loosely(t, out, should.ContainSubstring("Status:        FAILURE"))
			assert.Loosely(t, out, should.ContainSubstring("Flags:         -buildid 8738491827364512346 -invocationid build-8738491827364512346"))
			assert.Loosely(t, out, should.ContainSubstring("Timing:        18m 30s"))
			assert.Loosely(t, out, should.ContainSubstring("Summary(truncated): 1 test failed"))
			assert.Loosely(t, out, should.ContainSubstring("(Tip: To inspect build details, steps and full summaries, run: luci build get -buildid 8738491827364512346)"))
			assert.Loosely(t, out, should.ContainSubstring("luci verdict list -invocationid build-8738491827364512346"))
		})

		t.Run(`Successful build with custom ResultDB invocation and untruncated summary`, func(t *ftt.Test) {
			builds := []*pb.Build{
				{
					Id:     8738491827364512345,
					Number: 12345,
					Builder: &pb.BuilderID{
						Project: "chromium",
						Bucket:  "ci",
						Builder: "linux-rel",
					},
					Status:          pb.Status_SUCCESS,
					SummaryMarkdown: "All steps succeeded",
					Infra: &pb.BuildInfra{
						Resultdb: &pb.BuildInfra_ResultDB{
							Invocation: "invocations/u-custom-12345",
						},
					},
				},
			}

			var buf bytes.Buffer
			err := FormatBuildList(&buf, builds)
			assert.Loosely(t, err, should.BeNil)

			out := buf.String()
			assert.Loosely(t, out, should.ContainSubstring("Flags:         -buildid 8738491827364512345 -invocationid u-custom-12345"))
			assert.Loosely(t, out, should.ContainSubstring("Summary:       All steps succeeded"))
			assert.Loosely(t, out, should.ContainSubstring("(Tip: To inspect build details, steps and full summaries, run: luci build get -buildid 8738491827364512345)"))
			assert.Loosely(t, out, should.NotContainSubstring("luci verdict list"))
		})
	})
}

func TestBuildListRun(t *testing.T) {
	ftt.Run(`buildListRun`, t, func(t *ftt.Test) {
		af := base.NewAuthFlags()

		t.Run(`Missing filters returns 1`, func(t *ftt.Test) {
			cmd := ListCmd(af)
			run := cmd.CommandRun().(*buildListRun)
			ret := run.Run(nil, []string{}, nil)
			assert.Loosely(t, ret, should.Equal(1))
		})

		t.Run(`Positional arguments return 1`, func(t *ftt.Test) {
			cmd := ListCmd(af)
			run := cmd.CommandRun().(*buildListRun)
			ret := run.Run(nil, []string{"chromium/ci/linux-rel"}, nil)
			assert.Loosely(t, ret, should.Equal(1))
		})

		t.Run(`Non-positive limit returns 1`, func(t *ftt.Test) {
			cmd := ListCmd(af)
			run := cmd.CommandRun().(*buildListRun)
			err := run.Flags.Parse([]string{"-builder", "chromium/ci/linux-rel", "-n", "0"})
			assert.Loosely(t, err, should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(1))
		})

		t.Run(`Returns builds matching builder, tag, and status flags`, func(t *ftt.Test) {
			mock := &mockBuildsClient{
				searchBuilds: func(ctx context.Context, in *pb.SearchBuildsRequest, opts ...grpc.CallOption) (*pb.SearchBuildsResponse, error) {
					assert.Loosely(t, in.PageSize, should.Equal(int32(5)))
					assert.Loosely(t, in.Predicate, should.Match(&pb.BuildPredicate{
						Builder: &pb.BuilderID{
							Project: "chromium",
							Bucket:  "ci",
							Builder: "Linux UBSan Tests",
						},
						Tags: []*pb.StringPair{
							{Key: "dut-name", Value: "host1"},
						},
						Status: pb.Status_FAILURE,
					}))
					return &pb.SearchBuildsResponse{
						Builds: []*pb.Build{
							{
								Id:     8738491827364512346,
								Status: pb.Status_FAILURE,
							},
						},
					}, nil
				},
			}

			cmd := ListCmd(af)
			run := cmd.CommandRun().(*buildListRun)
			run.client = mock
			err := run.Flags.Parse([]string{
				"-builder", "chromium/ci/Linux UBSan Tests",
				"-t", "dut-name:host1",
				"-status", "failure",
				"-n", "5",
			})
			assert.Loosely(t, err, should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(0))
		})

		t.Run(`RPC error returns 1`, func(t *ftt.Test) {
			mock := &mockBuildsClient{
				searchBuilds: func(ctx context.Context, in *pb.SearchBuildsRequest, opts ...grpc.CallOption) (*pb.SearchBuildsResponse, error) {
					return nil, status.Errorf(codes.PermissionDenied, "access denied")
				},
			}

			cmd := ListCmd(af)
			run := cmd.CommandRun().(*buildListRun)
			run.client = mock
			err := run.Flags.Parse([]string{"-builder", "chromium/ci/linux-rel"})
			assert.Loosely(t, err, should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(1))
		})

		t.Run(`Returns builds for -cl URL with patchset`, func(t *ftt.Test) {
			mock := &mockBuildsClient{
				searchBuilds: func(ctx context.Context, in *pb.SearchBuildsRequest, opts ...grpc.CallOption) (*pb.SearchBuildsResponse, error) {
					assert.Loosely(t, in.Predicate.GerritChanges, should.Match([]*pb.GerritChange{
						{
							Host:     "chromium-review.googlesource.com",
							Project:  "infra/luci/luci-go",
							Change:   8565529,
							Patchset: 10,
						},
					}))
					return &pb.SearchBuildsResponse{
						Builds: []*pb.Build{
							{
								Id:     8738491827364512347,
								Status: pb.Status_SUCCESS,
							},
						},
					}, nil
				},
			}

			cmd := ListCmd(af)
			run := cmd.CommandRun().(*buildListRun)
			run.client = mock
			err := run.Flags.Parse([]string{
				"-cl", "https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/8565529/10",
			})
			assert.Loosely(t, err, should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(0))
		})

		t.Run(`Returns builds for latest patchset when omitted from -cl`, func(t *ftt.Test) {
			mockGerrit := &mockGerritClient{
				getChange: func(ctx context.Context, in *gerritpb.GetChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
					assert.Loosely(t, in.Number, should.Equal(8565529))
					return &gerritpb.ChangeInfo{
						Number:          8565529,
						Project:         "infra/luci/luci-go",
						CurrentRevision: "rev_8",
						Revisions: map[string]*gerritpb.RevisionInfo{
							"rev_8": {Number: 8},
						},
					}, nil
				},
			}

			mockBB := &mockBuildsClient{
				searchBuilds: func(ctx context.Context, in *pb.SearchBuildsRequest, opts ...grpc.CallOption) (*pb.SearchBuildsResponse, error) {
					assert.Loosely(t, in.Predicate.GerritChanges, should.Match([]*pb.GerritChange{
						{
							Host:     "chromium-review.googlesource.com",
							Project:  "infra/luci/luci-go",
							Change:   8565529,
							Patchset: 8,
						},
					}))
					return &pb.SearchBuildsResponse{
						Builds: []*pb.Build{
							{
								Id:     8738491827364512348,
								Status: pb.Status_SUCCESS,
							},
						},
					}, nil
				},
			}

			cmd := ListCmd(af)
			run := cmd.CommandRun().(*buildListRun)
			run.client = mockBB
			run.gerritClient = mockGerrit
			err := run.Flags.Parse([]string{
				"-cl", "https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/8565529",
			})
			assert.Loosely(t, err, should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(0))
		})

		t.Run(`Returns correct patchset with -patchset flag`, func(t *ftt.Test) {
			mockBB := &mockBuildsClient{
				searchBuilds: func(ctx context.Context, in *pb.SearchBuildsRequest, opts ...grpc.CallOption) (*pb.SearchBuildsResponse, error) {
					assert.Loosely(t, in.Predicate.GerritChanges, should.Match([]*pb.GerritChange{
						{
							Host:     "chromium-review.googlesource.com",
							Change:   8565529,
							Patchset: 4,
						},
					}))
					return &pb.SearchBuildsResponse{
						Builds: []*pb.Build{
							{
								Id:     8738491827364512349,
								Status: pb.Status_SUCCESS,
							},
						},
					}, nil
				},
			}

			cmd := ListCmd(af)
			run := cmd.CommandRun().(*buildListRun)
			run.client = mockBB
			err := run.Flags.Parse([]string{
				"-cl", "8565529",
				"-patchset", "4",
			})
			assert.Loosely(t, err, should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(0))
		})

		t.Run(`Invalid -cl returns 1`, func(t *ftt.Test) {
			cmd := ListCmd(af)
			run := cmd.CommandRun().(*buildListRun)
			err := run.Flags.Parse([]string{"-cl", "not-a-cl"})
			assert.Loosely(t, err, should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(1))
		})

		t.Run(`Gerrit patchset resolution error returns 1`, func(t *ftt.Test) {
			mockGerrit := &mockGerritClient{
				getChange: func(ctx context.Context, in *gerritpb.GetChangeRequest, opts ...grpc.CallOption) (*gerritpb.ChangeInfo, error) {
					return nil, status.Errorf(codes.NotFound, "CL not found")
				},
			}

			cmd := ListCmd(af)
			run := cmd.CommandRun().(*buildListRun)
			run.gerritClient = mockGerrit
			err := run.Flags.Parse([]string{"-cl", "8565529"})
			assert.Loosely(t, err, should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(1))
		})
	})
}
