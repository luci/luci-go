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

package control

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/server/span"

	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/testutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// EntryBuilder provides methods to build ingestion control records.
type EntryBuilder struct {
	record *Entry
}

// NewEntry starts building a new Entry.
func NewEntry(uniqifier int) *EntryBuilder {
	return &EntryBuilder{
		record: &Entry{
			IngestionID:         IngestionID(fmt.Sprintf("rdb-host/%v", uniqifier)),
			HasBuildBucketBuild: true,
			BuildProject:        "build-project",
			BuildResult: &controlpb.BuildResult{
				Host:         "buildbucket-host",
				Id:           int64(uniqifier),
				CreationTime: timestamppb.New(time.Date(2025, time.December, 1, 1, 2, 3, uniqifier*1000, time.UTC)),
				Project:      "myproject",
				Builder:      "builder",
				Status:       pb.BuildStatus_BUILD_STATUS_SUCCESS,
				Changelists: []*pb.Changelist{
					{
						Host:      "myhost-review.googlesource.com",
						Change:    123456,
						Patchset:  123,
						OwnerKind: pb.ChangelistOwnerKind_AUTOMATION,
					},
				},
				Commit: &buildbucketpb.GitilesCommit{
					Host:     "myproject.googlesource.com",
					Project:  "someproject/src",
					Id:       strings.Repeat("0a", 20),
					Ref:      "refs/heads/mybranch",
					Position: 111888,
				},
				HasInvocation: true,
				ResultdbHost:  "resultdb_host",
			},
			BuildJoinedTime:   time.Date(2020, time.December, 11, 1, 1, 1, uniqifier*1000, time.UTC),
			HasInvocation:     true,
			InvocationProject: "invocation-project",
			InvocationResult: &controlpb.InvocationResult{
				ResultdbHost: "rdb-host",
				InvocationId: "123",
			},
			InvocationJoinedTime: time.Date(2020, time.December, 12, 1, 1, 1, uniqifier*1000, time.UTC),
			IsPresubmit:          true,
			PresubmitProject:     "presubmit-project",
			PresubmitResult: &controlpb.PresubmitResult{
				PresubmitRunId: &pb.PresubmitRunId{
					System: "luci-cv",
					Id:     fmt.Sprintf("%s/123123-%v", "presubmit-project", uniqifier),
				},
				Status:       pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_SUCCEEDED,
				Mode:         pb.PresubmitRunMode_QUICK_DRY_RUN,
				Owner:        "automation",
				CreationTime: timestamppb.New(time.Date(2026, time.December, 1, 1, 2, 3, uniqifier*1000, time.UTC)),
			},
			PresubmitJoinedTime: time.Date(2020, time.December, 13, 1, 1, 1, uniqifier*1000, time.UTC),
			LastUpdated:         time.Date(2020, time.December, 14, 1, 1, 1, uniqifier*1000, time.UTC),
		},
	}
}

// WithIngestionID specifies the ingestion ID to use on the ingestion control record.
func (b *EntryBuilder) WithIngestionID(id IngestionID) *EntryBuilder {
	b.record.IngestionID = id
	return b
}

// WithBuildProject specifies the build project to use on the ingestion control record.
func (b *EntryBuilder) WithBuildProject(project string) *EntryBuilder {
	b.record.BuildProject = project
	return b
}

// WithBuildResult specifies the build result for the entry.
func (b *EntryBuilder) WithBuildResult(value *controlpb.BuildResult) *EntryBuilder {
	b.record.BuildResult = value
	return b
}

// WithBuildJoinedTime specifies the time the build result was populated.
func (b *EntryBuilder) WithBuildJoinedTime(value time.Time) *EntryBuilder {
	b.record.BuildJoinedTime = value
	return b
}

// WithHasInvocation specifies whether the build that is the subject of the ingestion
// has a ResultDB invocation.
func (b *EntryBuilder) WithHasInvocation(hasInvocation bool) *EntryBuilder {
	b.record.HasInvocation = hasInvocation
	return b
}

// WithHasBuildbucketBuild specifies whether the build that is the subject of the ingestion
// has a buildbucket build.
func (b *EntryBuilder) WithHasBuildbucketBuild(hasBuildBucketBuild bool) *EntryBuilder {
	b.record.HasBuildBucketBuild = hasBuildBucketBuild
	return b
}

// WithInvocationProject specifies the invocation project to use on the ingestion control record.
func (b *EntryBuilder) WithInvocationProject(project string) *EntryBuilder {
	b.record.InvocationProject = project
	return b
}

// WithInvocationResult specifies the invocation result for the entry.
func (b *EntryBuilder) WithInvocationResult(value *controlpb.InvocationResult) *EntryBuilder {
	b.record.InvocationResult = value
	return b
}

// WithInvocationJoinedTime specifies the time the invocation result was populated.
func (b *EntryBuilder) WithInvocationJoinedTime(value time.Time) *EntryBuilder {
	b.record.InvocationJoinedTime = value
	return b
}

// WithIsPresubmit specifies whether the ingestion relates to a presubmit run.
func (b *EntryBuilder) WithIsPresubmit(isPresubmit bool) *EntryBuilder {
	b.record.IsPresubmit = isPresubmit
	return b
}

// WithPresubmitProject specifies the presubmit project to use on the ingestion control record.
func (b *EntryBuilder) WithPresubmitProject(project string) *EntryBuilder {
	b.record.PresubmitProject = project
	return b
}

// WithPresubmitResult specifies the build result for the entry.
func (b *EntryBuilder) WithPresubmitResult(value *controlpb.PresubmitResult) *EntryBuilder {
	b.record.PresubmitResult = value
	return b
}

// WithPresubmitJoinedTime specifies the time the presubmit result was populated.
func (b *EntryBuilder) WithPresubmitJoinedTime(lastUpdated time.Time) *EntryBuilder {
	b.record.PresubmitJoinedTime = lastUpdated
	return b
}

// Build constructs the entry.
func (b *EntryBuilder) Build() *Entry {
	return b.record
}

// SetEntriesForTesting replaces the set of stored entries to match the given set.
func SetEntriesForTesting(ctx context.Context, t testing.TB, es ...*Entry) (time.Time, error) {
	t.Helper()
	testutil.MustApply(ctx, t,
		spanner.Delete("IngestionJoins", spanner.AllKeys()))
	// Insert some Ingestion records.
	commitTime, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		for _, r := range es {
			ms := spanutil.InsertMap("IngestionJoins", map[string]any{
				"IngestionID":          string(r.IngestionID),
				"HasBuildBucketBuild":  r.HasBuildBucketBuild,
				"BuildProject":         r.BuildProject,
				"BuildResult":          r.BuildResult,
				"BuildJoinedTime":      r.BuildJoinedTime,
				"HasInvocation":        r.HasInvocation,
				"InvocationProject":    r.InvocationProject,
				"InvocationResult":     r.InvocationResult,
				"InvocationJoinedTime": r.InvocationJoinedTime,
				"IsPresubmit":          r.IsPresubmit,
				"PresubmitProject":     r.PresubmitProject,
				"PresubmitResult":      r.PresubmitResult,
				"PresubmitJoinedTime":  r.PresubmitJoinedTime,
				"LastUpdated":          r.LastUpdated,
			})
			span.BufferWrite(ctx, ms)
		}
		return nil
	})
	return commitTime.In(time.UTC), err
}
