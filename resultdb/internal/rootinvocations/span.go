// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package rootinvocations defines functions to interact with spanner.
package rootinvocations

import (
	"fmt"
	"regexp"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	// secondaryIndexShardCount is the number of values for the SecondaryIndexShardId column in the RootInvocations table
	// used for preventing hotspots in global secondary indexes. The range of values is [0, secondaryIndexShardCount-1].
	secondaryIndexShardCount = 100
)

// Create returns mutations to create the following records.
//   - a new root invocation record,
//   - the corresponding legacy invocation record
//   - sharding records in RootInvocationShards
func Create(rootInvocation *RootInvocationRow) []*spanner.Mutation {
	if rootInvocation.RootInvocationID == "" {
		panic("do not create root invocations with empty id")
	}
	if rootInvocation.FinalizationState != pb.RootInvocation_FINALIZING && rootInvocation.FinalizationState != pb.RootInvocation_ACTIVE {
		// validateCreateInvocationRequest should have rejected any other states.
		panic("do not create root invocations in states other than active or finalizing")
	}
	if rootInvocation.Realm == "" {
		panic("do not create root invocations with empty realm")
	}
	if rootInvocation.CreatedBy == "" {
		panic("do not create root invocations with empty creator")
	}
	if rootInvocation.Deadline.IsZero() {
		panic("do not create root invocations with empty deadline")
	}
	if !rootInvocation.UninterestingTestVerdictsExpirationTime.Valid {
		panic("do not create root invocations with empty UninterestingTestVerdictsExpirationTime")
	}
	rootInvocation.Normalize()
	// Create mutation for the RootInvocations table record.
	rootInvMutation := rootInvocation.toMutation()

	// Create mutation for the legacy invocation table record.
	invMutation := rootInvocation.toLegacyInvocationMutation()

	// Create mutation for the RootInvocationShards table records.
	shardMutations := rootInvocation.toShardsMutations()

	mutations := []*spanner.Mutation{rootInvMutation, invMutation}
	mutations = append(mutations, shardMutations...)
	return mutations
}

// RootInvocationRow corresponds to the schema of the RootInvocations Spanner table.
// The values for the output only fields are ignored during writing.
type RootInvocationRow struct {
	RootInvocationID                        ID
	SecondaryIndexShardID                   int64 // Output only.
	FinalizationState                       pb.RootInvocation_FinalizationState
	Realm                                   string
	CreateTime                              time.Time // Output only.
	CreatedBy                               string    // Output only.
	LastUpdated                             time.Time
	FinalizeStartTime                       spanner.NullTime // Output only.
	FinalizeTime                            spanner.NullTime // Output only.
	Deadline                                time.Time
	UninterestingTestVerdictsExpirationTime spanner.NullTime
	CreateRequestID                         string
	ProducerResource                        string
	Tags                                    []*pb.StringPair
	Properties                              *structpb.Struct
	Sources                                 *pb.Sources
	IsSourcesFinal                          bool
	BaselineID                              string
	Submitted                               bool
	FinalizerPending                        bool
	FinalizerSequence                       int64
}

// Clone makes a deep copy of the row.
func (r *RootInvocationRow) Clone() *RootInvocationRow {
	ret := *r
	if r.Tags != nil {
		ret.Tags = make([]*pb.StringPair, len(r.Tags))
		for i, tp := range r.Tags {
			ret.Tags[i] = proto.Clone(tp).(*pb.StringPair)
		}
	}
	if r.Properties != nil {
		ret.Properties = proto.Clone(r.Properties).(*structpb.Struct)
	}
	if r.Sources != nil {
		ret.Sources = proto.Clone(r.Sources).(*pb.Sources)
	}
	return &ret
}

// Convert the root invocation row to the canonical form.
func (r *RootInvocationRow) Normalize() {
	pbutil.SortStringPairs(r.Tags)

	changelists := r.Sources.GetChangelists()
	pbutil.SortGerritChanges(changelists)
}

func (r *RootInvocationRow) toMutation() *spanner.Mutation {
	row := map[string]interface{}{
		"RootInvocationId":      r.RootInvocationID,
		"SecondaryIndexShardId": r.RootInvocationID.shardID(secondaryIndexShardCount),
		"FinalizationState":     r.FinalizationState,
		"State":                 pb.RootInvocation_STATE_UNSPECIFIED,
		"Realm":                 r.Realm,
		"CreateTime":            spanner.CommitTimestamp,
		"CreatedBy":             r.CreatedBy,
		"LastUpdated":           spanner.CommitTimestamp,
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
		"FinalizerPending":                        r.FinalizerPending,
		"FinalizerSequence":                       r.FinalizerSequence,
	}

	if r.FinalizationState == pb.RootInvocation_FINALIZING {
		// Invocation immediately transitioning to finalizing.
		row["FinalizeStartTime"] = spanner.CommitTimestamp
	}
	return spanutil.InsertMap("RootInvocations", row)
}

func (r *RootInvocationRow) toLegacyInvocationMutation() *spanner.Mutation {
	row := map[string]interface{}{
		"InvocationId":                      r.RootInvocationID.LegacyInvocationID(),
		"Type":                              invocations.Root,
		"ShardId":                           r.RootInvocationID.shardID(invocations.Shards),
		"State":                             toInvocationState(r.FinalizationState),
		"Realm":                             r.Realm,
		"InvocationExpirationTime":          time.Unix(0, 0), // unused field, but spanner schema enforce it to be not null.
		"ExpectedTestResultsExpirationTime": r.UninterestingTestVerdictsExpirationTime,
		"CreateTime":                        spanner.CommitTimestamp,
		"CreatedBy":                         r.CreatedBy,
		"Deadline":                          r.Deadline,
		"Tags":                              r.Tags,
		"CreateRequestId":                   r.CreateRequestID,
		"ProducerResource":                  r.ProducerResource,
		"Properties":                        spanutil.Compressed(pbutil.MustMarshal(r.Properties)),
		"InheritSources":                    spanner.NullBool{Bool: false, Valid: true}, // A root invocation defines its own sources.
		"Sources":                           spanutil.Compressed(pbutil.MustMarshal(r.Sources)),
		"IsSourceSpecFinal":                 r.IsSourcesFinal,
		"IsExportRoot":                      spanner.NullBool{Bool: true, Valid: true}, // Root invocations are always export roots.
		"BaselineId":                        r.BaselineID,
		"Submitted":                         r.Submitted,
	}

	if r.FinalizationState == pb.RootInvocation_FINALIZING {
		// Invocation immediately transitioning to finalizing.
		row["FinalizeStartTime"] = spanner.CommitTimestamp
	}
	return spanutil.InsertMap("Invocations", row)
}

func (r *RootInvocationRow) toShardsMutations() []*spanner.Mutation {
	mutations := make([]*spanner.Mutation, RootInvocationShardCount)
	for i := 0; i < RootInvocationShardCount; i++ {
		row := map[string]interface{}{
			"RootInvocationShardId": ShardID{RootInvocationID: r.RootInvocationID, ShardIndex: i},
			"ShardIndex":            i,
			"RootInvocationId":      r.RootInvocationID,
			"Realm":                 r.Realm,
			"CreateTime":            spanner.CommitTimestamp,
			"Sources":               spanutil.Compressed(pbutil.MustMarshal(r.Sources)),
			"IsSourcesFinal":        r.IsSourcesFinal,
		}
		mutations[i] = spanutil.InsertMap("RootInvocationShards", row)
	}
	return mutations
}

func (r *RootInvocationRow) ToProto() *pb.RootInvocation {
	result := &pb.RootInvocation{
		Name:              r.RootInvocationID.Name(),
		RootInvocationId:  string(r.RootInvocationID),
		FinalizationState: r.FinalizationState,
		Realm:             r.Realm,
		CreateTime:        pbutil.MustTimestampProto(r.CreateTime),
		Creator:           r.CreatedBy,
		LastUpdated:       pbutil.MustTimestampProto(r.LastUpdated),
		Deadline:          pbutil.MustTimestampProto(r.Deadline),
		ProducerResource:  r.ProducerResource,
		Sources:           r.Sources,
		SourcesFinal:      r.IsSourcesFinal,
		Tags:              r.Tags,
		Properties:        r.Properties,
		BaselineId:        r.BaselineID,
		Etag:              Etag(r),
	}
	if r.FinalizeStartTime.Valid {
		result.FinalizeStartTime = pbutil.MustTimestampProto(r.FinalizeStartTime.Time)
	}
	if r.FinalizeTime.Valid {
		result.FinalizeTime = pbutil.MustTimestampProto(r.FinalizeTime.Time)
	}
	return result
}

func toInvocationState(finalizationState pb.RootInvocation_FinalizationState) pb.Invocation_State {
	switch finalizationState {
	case pb.RootInvocation_ACTIVE:
		return pb.Invocation_ACTIVE
	case pb.RootInvocation_FINALIZING:
		return pb.Invocation_FINALIZING
	case pb.RootInvocation_FINALIZED:
		return pb.Invocation_FINALIZED
	default:
		panic(fmt.Sprintf("unknown root invocation state %s", finalizationState))
	}
}

func (r *RootInvocationRow) ToLegacyInvocationProto() *pb.Invocation {
	var sourceSpec *pb.SourceSpec
	if r.Sources != nil {
		sourceSpec = &pb.SourceSpec{Sources: r.Sources}
	}
	result := &pb.Invocation{
		Name:                   r.RootInvocationID.LegacyInvocationID().Name(),
		State:                  toInvocationState(r.FinalizationState),
		Realm:                  r.Realm,
		CreateTime:             pbutil.MustTimestampProto(r.CreateTime),
		CreatedBy:              r.CreatedBy,
		Deadline:               pbutil.MustTimestampProto(r.Deadline),
		ProducerResource:       r.ProducerResource,
		SourceSpec:             sourceSpec,
		IsSourceSpecFinal:      r.IsSourcesFinal,
		Tags:                   r.Tags,
		IsExportRoot:           true,
		Properties:             r.Properties,
		BaselineId:             r.BaselineID,
		TestResultVariantUnion: &pb.Variant{},
	}
	if r.FinalizeStartTime.Valid {
		result.FinalizeStartTime = pbutil.MustTimestampProto(r.FinalizeStartTime.Time)
	}
	if r.FinalizeTime.Valid {
		result.FinalizeTime = pbutil.MustTimestampProto(r.FinalizeTime.Time)
	}
	return result
}

// Etag returns the HTTP ETag for the given root invocation.
func Etag(r *RootInvocationRow) string {
	// The ETag must be a function of the resource representation according to (AIP-154).
	return fmt.Sprintf(`W/"%s"`, r.LastUpdated.UTC().Format(time.RFC3339Nano))
}

// etagRegexp extracts the root invocation's last updated timestamp from a root invocation ETag.
var etagRegexp = regexp.MustCompile(`^W/"(.*)"$`)

// ParseEtag validate the etag and returns the embedded lastUpdated time.
func ParseEtag(etag string) (lastUpdated string, err error) {
	m := etagRegexp.FindStringSubmatch(etag)
	if len(m) < 2 {
		return "", errors.Fmt("malformated etag")
	}
	return m[1], nil
}

// IsEtagMatch determines if the Etag is consistent with the specified
// root invocation version.
func IsEtagMatch(r *RootInvocationRow, etag string) (bool, error) {
	lastUpdated, err := ParseEtag(etag)
	if err != nil {
		return false, err
	}
	return lastUpdated == r.LastUpdated.UTC().Format(time.RFC3339Nano), nil
}

// MarkFinalizing creates mutations to mark the given root invocation as finalizing.
// The caller MUST check the root invocation is currently in ACTIVE state, or this
// may incorrectly overwrite the FinalizeStartTime.
func MarkFinalizing(id ID) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, 0, 2)
	ms = append(ms, spanutil.UpdateMap("RootInvocations", map[string]any{
		"RootInvocationId":  id,
		"FinalizationState": pb.RootInvocation_FINALIZING,
		"LastUpdated":       spanner.CommitTimestamp,
		"FinalizeStartTime": spanner.CommitTimestamp,
	}))
	ms = append(ms, invocations.MarkFinalizing(id.LegacyInvocationID()))
	return ms
}

// MarkFinalized creates a mutation to mark the given root invocation as finalized.
// The caller MUST check the root invocation is currently in FINALIZING state, or this
// may incorrectly overwrite the FinalizeTime.
func MarkFinalized(id ID) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, 0, 2)
	ms = append(ms, spanutil.UpdateMap("RootInvocations", map[string]any{
		"RootInvocationId":  id,
		"FinalizationState": pb.RootInvocation_FINALIZED,
		"LastUpdated":       spanner.CommitTimestamp,
		"FinalizeTime":      spanner.CommitTimestamp,
	}))
	ms = append(ms, invocations.MarkFinalized(id.LegacyInvocationID()))
	return ms
}

func CreateRootInvocationUpdateRequest(id ID, updatedBy, requestID string) *spanner.Mutation {
	return spanutil.InsertMap("RootInvocationUpdateRequests", map[string]interface{}{
		"RootInvocationId": id,
		"UpdatedBy":        updatedBy,
		"UpdateRequestId":  requestID,
		"CreateTime":       spanner.CommitTimestamp,
	})
}

// SetFinalizerPending returns mutations to set FinalizerPending to true,
// and sets the new sequence number.
// Note: The LastUpdated time is not updated because FinalizerPending and
// FinalizerSequence are internal control fields for managing background tasks.
func SetFinalizerPending(id ID, newSeq int64) *spanner.Mutation {
	return spanutil.UpdateMap("RootInvocations", map[string]any{
		"RootInvocationId":  id,
		"FinalizerPending":  true,
		"FinalizerSequence": newSeq,
	})
}

// ResetFinalizerPending returns a mutation which set FinalizerPending to false.
// Note: The LastUpdated time is not updated because FinalizerPending is an
// internal control field for managing background tasks.
func ResetFinalizerPending(id ID) *spanner.Mutation {
	return spanutil.UpdateMap("RootInvocations", map[string]any{
		"RootInvocationId": id,
		"FinalizerPending": false,
	})
}
