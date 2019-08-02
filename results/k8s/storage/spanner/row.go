// Copyright 2019 The LUCI Authors.
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

package spanner

import (
	"math/rand"
	"time"

	gspan "cloud.google.com/go/spanner"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
)

var testResultCounterShardFactor = int64(64)

// Row defines an interface for inserting and updating Spanner LUCI results rows.
type Row interface {
	// GetTable returns the table in which this row lives.
	GetTable() string

	// GetWriteMutations gets the mutations for inserting this row in the given transaction.
	GetWriteMutations(ctx context.Context, txn *gspan.ReadWriteTransaction) ([]*gspan.Mutation, error)
}

// InvocationRow represents a struct to be inserted into the Invocations table.
type InvocationRow struct {
	InvocationId string
	Realm        string

	InvocationExpirationTime          time.Time
	InvocationExpirationWeek          time.Time
	ExpectedTestResultsExpirationTime time.Time
	ExpectedTestResultsExpirationWeek time.Time

	CreateTime   gspan.NullTime
	FinalizeTime gspan.NullTime
	Deadline     gspan.NullTime

	Tags []string
}

func (row InvocationRow) GetTable() string { return "Invocations" }

func (row InvocationRow) GetWriteMutations(ctx context.Context, txn *gspan.ReadWriteTransaction) ([]*gspan.Mutation, error) {
	return convertStructToSingleMut(ctx, row)
}

// VariantDefRow represents a struct to be inserted into the VariantDefs table.
type VariantDefRow struct {
	VariantId string
	Pairs     []string // limit 16 pairs
}

func (row VariantDefRow) GetTable() string { return "VariantDefs" }

func (row VariantDefRow) GetWriteMutations(ctx context.Context, txn *gspan.ReadWriteTransaction) ([]*gspan.Mutation, error) {
	return convertStructToSingleMut(ctx, row)
}

// InclusionRow represents a struct to be inserted into the Inclusions table.
type InclusionRow struct {
	InvocationId         string
	IncludedInvocationId string

	Consequential bool
	Ready         bool
}

func (row InclusionRow) GetTable() string { return "Inclusions" }

func (row InclusionRow) GetWriteMutations(ctx context.Context, txn *gspan.ReadWriteTransaction) ([]*gspan.Mutation, error) {
	return convertStructToSingleMut(ctx, row)
}

// InvocationsByTagRow represents a struct to be inserted into the InvocationsByTags table.
type InvocationsByTagRow struct {
	Tag          string
	InvocationId string
}

func (row InvocationsByTagRow) GetTable() string { return "InvocationsByTag" }

func (row InvocationsByTagRow) GetWriteMutations(ctx context.Context, txn *gspan.ReadWriteTransaction) ([]*gspan.Mutation, error) {
	return convertStructToSingleMut(ctx, row)
}

// ExonerationRow represents a struct to be inserted into the Exonerations table.
type ExonerationRow struct {
	InvocationId  string
	TestPath      string
	VariantId     string
	ExonerationId int64

	MarkdownExplanation string

	// Below fields should be left empty by user, and will be populated on insert.
	LastUpdateTime time.Time
}

func (row ExonerationRow) GetTable() string { return "Exonerations" }

func (row ExonerationRow) GetWriteMutations(ctx context.Context, txn *gspan.ReadWriteTransaction) ([]*gspan.Mutation, error) {
	// Do not allow manually provided update commit timestamp.
	if !row.LastUpdateTime.Equal(time.Time{}) {
		return nil, errors.Reason("manual commit timestamps disallowed").Err()
	}

	row.LastUpdateTime = gspan.CommitTimestamp
	mut, err := gspan.InsertOrUpdateStruct(row.GetTable(), row)
	if err != nil {
		return nil, err
	}
	return []*gspan.Mutation{mut}, nil
}

// TestResultRow represents a struct to be inserted in to the TestResults table.
//
// Insertion thereof requires insertion into the TestResultCounters table, which is handled by a
//  helper struct.
type TestResultRow struct {
	InvocationId  string
	TestPath      string
	ResultId      int64
	BaseVariantId string
	VariantPairs  []string

	IsUnexpected    bool
	Status          int64
	MarkdownSummary string

	StartTime     time.Time
	RunDurationMs int64

	Tags  []string
	Files []byte

	// Below fields should be left empty by user, and will be populated on insert.
	CommitTimestamp time.Time
}

func (row TestResultRow) GetTable() string { return "TestResults" }

func (row TestResultRow) GetWriteMutations(ctx context.Context, txn *gspan.ReadWriteTransaction) ([]*gspan.Mutation, error) {
	// Do not allow manually provided update commit timestamp.
	if !row.CommitTimestamp.Equal(time.Time{}) {
		return nil, errors.Reason("manual commit timestamps disallowed").Err()
	}

	row.CommitTimestamp = gspan.CommitTimestamp
	resultMut, err := gspan.InsertOrUpdateStruct(row.GetTable(), row)
	if err != nil {
		return nil, err
	}

	shardRow := testResultCounterRow{
		InvocationId: row.InvocationId,
		CounterShard: rand.Int63n(testResultCounterShardFactor),
	}

	count, err := txn.ReadRow(
		ctx,
		shardRow.GetTable(),
		gspan.Key{shardRow.InvocationId, shardRow.CounterShard},
		[]string{"TestResultCount"},
	)

	switch {
	case err == nil:
		if err := count.Column(0, &shardRow.TestResultCount); err != nil {
			return nil, err
		}
	case gspan.ErrCode(err) == codes.NotFound:
		shardRow.TestResultCount = 0
	default:
		return nil, err
	}
	shardRow.TestResultCount++

	shardMut, err := gspan.InsertOrUpdateStruct(shardRow.GetTable(), shardRow)
	if err != nil {
		return nil, err
	}

	return []*gspan.Mutation{resultMut, shardMut}, nil
}

// Helper struct representing rows for sharding TestResults that should be not used directly.
type testResultCounterRow struct {
	InvocationId    string
	CounterShard    int64
	TestResultCount int64
}

func (row testResultCounterRow) GetTable() string { return "TestResultCounters" }

// Helper function for converting a struct directly to a slice of a single mutation.
func convertStructToSingleMut(ctx context.Context, row Row) ([]*gspan.Mutation, error) {
	mut, err := gspan.InsertOrUpdateStruct(row.GetTable(), row)
	if err != nil {
		return nil, err
	}
	return []*gspan.Mutation{mut}, nil
}
