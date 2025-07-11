// Copyright 2025 The LUCI Authors.
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

package rootinvocations

import (
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Builder is a builder for RootInvocationRow for testing.
type Builder struct {
	row RootInvocationRow
}

// NewBuilder returns a new builder for a RootInvocationRow for testing.
// The builder is initialized with some default values.
func NewBuilder(id ID) *Builder {
	return &Builder{
		row: RootInvocationRow{
			// Set all fields by default. This helps optimise test coverage.
			RootInvocationID:                        id,
			SecondaryIndexShardID:                   id.shardID(secondaryIndexShardCount),
			State:                                   pb.RootInvocation_FINALIZED,
			Realm:                                   "testproject:testrealm",
			CreateTime:                              time.Date(2025, 4, 25, 1, 2, 3, 4000, time.UTC),
			CreatedBy:                               "user:test@example.com",
			FinalizeStartTime:                       spanner.NullTime{Valid: true, Time: time.Date(2025, 4, 26, 1, 2, 3, 4000, time.UTC)},
			FinalizeTime:                            spanner.NullTime{Valid: true, Time: time.Date(2025, 4, 27, 1, 2, 3, 4000, time.UTC)},
			Deadline:                                time.Date(2025, 4, 28, 1, 2, 3, 4000, time.UTC),
			UninterestingTestVerdictsExpirationTime: spanner.NullTime{Valid: true, Time: time.Date(2025, 6, 28, 1, 2, 3, 4000, time.UTC)},
			CreateRequestID:                         "test-request-id",
			ProducerResource:                        "//builds.example.com/builds/123",
			Tags:                                    pbutil.StringPairs("k1", "v1"),
			Properties: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"key": structpb.NewStringValue("value"),
				},
			},
			Sources: &pb.Sources{
				GitilesCommit: &pb.GitilesCommit{
					Host:       "chromium.googlesource.com",
					Project:    "chromium/src",
					Ref:        "refs/heads/main",
					CommitHash: "1234567890abcdef1234567890abcdef12345678",
					Position:   12345,
				},
			},
			IsSourcesFinal: true,
			BaselineID:     "baseline",
			Submitted:      true,
		},
	}
}

// WithMinimalFields clears as many fields as possible on the root invocation while
// keeping it valid. This is useful for testing null and empty value handling.
func (b *Builder) WithMinimalFields() *Builder {
	b.row = RootInvocationRow{
		RootInvocationID:      b.row.RootInvocationID,
		SecondaryIndexShardID: b.row.SecondaryIndexShardID,
		// Means the finalized time and start time will be cleared in Build() unless state is
		// subsequently overridden.
		State:             pb.RootInvocation_ACTIVE,
		Realm:             b.row.Realm,
		CreateTime:        b.row.CreateTime,
		CreatedBy:         b.row.CreatedBy,
		FinalizeStartTime: b.row.FinalizeStartTime,
		FinalizeTime:      b.row.FinalizeTime,
		Deadline:          b.row.Deadline,
		CreateRequestID:   b.row.CreateRequestID,
		// Prefer to use empty slice rather than nil (even though semantically identical)
		// as this what we always report in reads.
		Tags: []*pb.StringPair{},
	}
	return b
}

// WithRootInvocationID sets the root invocation ID.
func (b *Builder) WithRootInvocationID(id ID) *Builder {
	b.row.RootInvocationID = id
	b.row.SecondaryIndexShardID = id.shardID(secondaryIndexShardCount)
	return b
}

// WithState sets the state of the root invocation.
func (b *Builder) WithState(state pb.RootInvocation_State) *Builder {
	b.row.State = state
	return b
}

// WithRealm sets the realm of the root invocation.
func (b *Builder) WithRealm(realm string) *Builder {
	b.row.Realm = realm
	return b
}

// WithCreateTime sets the creation time of the root invocation.
func (b *Builder) WithCreateTime(t time.Time) *Builder {
	b.row.CreateTime = t
	return b
}

// WithCreatedBy sets the creator of the root invocation.
func (b *Builder) WithCreatedBy(creator string) *Builder {
	b.row.CreatedBy = creator
	return b
}

// WithFinalizeStartTime sets the finalize start time.
func (b *Builder) WithFinalizeStartTime(t time.Time) *Builder {
	b.row.FinalizeStartTime = spanner.NullTime{Valid: true, Time: t}
	return b
}

// WithFinalizeTime sets the finalize time.
func (b *Builder) WithFinalizeTime(t time.Time) *Builder {
	b.row.FinalizeTime = spanner.NullTime{Valid: true, Time: t}
	return b
}

// WithDeadline sets the deadline of the root invocation.
func (b *Builder) WithDeadline(t time.Time) *Builder {
	b.row.Deadline = t
	return b
}

// WithUninterestingTestVerdictsExpirationTime sets the expiration time for uninteresting test verdicts.
func (b *Builder) WithUninterestingTestVerdictsExpirationTime(t spanner.NullTime) *Builder {
	b.row.UninterestingTestVerdictsExpirationTime = t
	return b
}

// WithCreateRequestID sets the create request ID.
func (b *Builder) WithCreateRequestID(id string) *Builder {
	b.row.CreateRequestID = id
	return b
}

// WithProducerResource sets the producer resource.
func (b *Builder) WithProducerResource(res string) *Builder {
	b.row.ProducerResource = res
	return b
}

// WithTags sets the tags.
func (b *Builder) WithTags(tags []*pb.StringPair) *Builder {
	b.row.Tags = tags
	return b
}

// WithProperties sets the properties.
func (b *Builder) WithProperties(p *structpb.Struct) *Builder {
	b.row.Properties = p
	return b
}

// WithSources sets the sources.
func (b *Builder) WithSources(s *pb.Sources) *Builder {
	b.row.Sources = s
	return b
}

// WithIsSourcesFinal sets whether sources are final.
func (b *Builder) WithIsSourcesFinal(isFinal bool) *Builder {
	b.row.IsSourcesFinal = isFinal
	return b
}

// WithBaselineID sets the baseline ID.
func (b *Builder) WithBaselineID(id string) *Builder {
	b.row.BaselineID = id
	return b
}

// WithSubmitted sets the submitted status.
func (b *Builder) WithSubmitted(submitted bool) *Builder {
	b.row.Submitted = submitted
	return b
}

// Build returns the constructed RootInvocationRow.
func (b *Builder) Build() *RootInvocationRow {
	// Return a copy to avoid changes to the returned row
	// flowing back into the builder.
	r := b.row.Clone()

	if r.State == pb.RootInvocation_ACTIVE {
		r.FinalizeStartTime = spanner.NullTime{}
		r.FinalizeTime = spanner.NullTime{}
	}
	if r.State == pb.RootInvocation_FINALIZING {
		r.FinalizeTime = spanner.NullTime{}
	}
	return r
}

// InsertForTesting inserts the rootInvocation record and all the
// RootInvocationShards records for a root invocation for testing purposes.
func InsertForTesting(r *RootInvocationRow) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, 0, 16+1) // 16 shard and 1 root invocation
	ms = append(ms, spanutil.InsertMap("RootInvocations", map[string]any{
		"RootInvocationId":      r.RootInvocationID,
		"SecondaryIndexShardId": r.SecondaryIndexShardID,
		"State":                 r.State,
		"Realm":                 r.Realm,
		"CreateTime":            r.CreateTime,
		"CreatedBy":             r.CreatedBy,
		"FinalizeStartTime":     r.FinalizeStartTime,
		"FinalizeTime":          r.FinalizeTime,
		"Deadline":              r.Deadline,
		"UninterestingTestVerdictsExpirationTime": r.UninterestingTestVerdictsExpirationTime,
		"CreateRequestId":                         r.CreateRequestID,
		"ProducerResource":                        r.ProducerResource,
		"Tags":                                    r.Tags,
		"Properties":                              spanutil.Compressed(pbutil.MustMarshal(r.Properties)),
		"Sources":                                 spanutil.Compressed(pbutil.MustMarshal(r.Sources)),
		"IsSourcesFinal":                          r.IsSourcesFinal,
		"BaselineId":                              r.BaselineID,
		"Submitted":                               r.Submitted,
	}))

	for i := 0; i < 16; i++ {
		ms = append(ms, spanutil.InsertMap("RootInvocationShards", map[string]any{
			"RootInvocationShardId": ShardID{RootInvocationID: r.RootInvocationID, ShardIndex: i},
			"ShardIndex":            i,
			"RootInvocationId":      r.RootInvocationID,
			"State":                 r.State,
			"Realm":                 r.Realm,
			"CreateTime":            r.CreateTime,
			"Sources":               spanutil.Compressed(pbutil.MustMarshal(r.Sources)),
			"IsSourcesFinal":        r.IsSourcesFinal,
		}))
	}
	return ms
}
