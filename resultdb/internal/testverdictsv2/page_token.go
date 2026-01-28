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

package testverdictsv2

import (
	"fmt"
	"strconv"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testexonerationsv2"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// PageToken is a page token for a query. It represents the last row retrieved.
// Not all fields are used for all queries, but for consistency all are populated.
type PageToken struct {
	// The UI priority of the last row. This is a value between 0 and 100, where
	// 0 is the highest priority and 100 is the lowest priority.
	UIPriority int64
	// The primary key of the last returned verdict.
	ID testresultsv2.VerdictID
	// The one-based index into QueryDetails.VerdictIDs that represents the last
	// returned verdict.
	// Used to keep position in case of duplicated ID(s) in QueryDetails.VerdictIDs.
	//
	// Only set for QueryDetails queries, if QueryDetails.VerdictIDs is set.
	// Otherwise, this is set to zero.
	RequestOrdinal int
}

// ParsePageToken parses a page token into a PageToken.
func ParsePageToken(token string) (PageToken, error) {
	if token == "" {
		// If provided to a request, indicates first page.
		// If returned in a response, indicates there are no more results.
		return PageToken{}, nil
	}
	parts, err := pagination.ParseToken(token)
	if err != nil {
		return PageToken{}, errors.Fmt("parse: %s", err)
	}
	const expectedParts = 10
	if len(parts) != expectedParts {
		return PageToken{}, errors.Fmt("expected %v components, got %d", expectedParts, len(parts))
	}
	uiPriority, err := strconv.Atoi(parts[0])
	if err != nil {
		return PageToken{}, errors.Fmt("got non-integer UIPriority: %v", parts[0])
	}
	if uiPriority > MaxUIPriority {
		return PageToken{}, errors.New("client modified token or logic error: uiPriority > maxUIPriority")
	}
	shardIndex, err := strconv.Atoi(parts[2])
	if err != nil {
		return PageToken{}, errors.Fmt("got non-integer shard index: %v", parts[2])
	}
	requestOrdinal, err := strconv.Atoi(parts[9])
	if err != nil {
		return PageToken{}, errors.Fmt("got non-integer request ordinal: %v", parts[9])
	}

	return PageToken{
		UIPriority: int64(uiPriority),
		ID: testresultsv2.VerdictID{
			RootInvocationShardID: rootinvocations.ShardID{
				RootInvocationID: rootinvocations.ID(parts[1]),
				ShardIndex:       shardIndex,
			},
			ModuleName:        parts[3],
			ModuleScheme:      parts[4],
			ModuleVariantHash: parts[5],
			CoarseName:        parts[6],
			FineName:          parts[7],
			CaseName:          parts[8],
		},
		RequestOrdinal: requestOrdinal,
	}, nil
}

// Serialize serializes a PageToken into a page token ready
// for an RPC response.
func (t PageToken) Serialize() string {
	if t == (PageToken{}) {
		// If provided to a request, indicates first page.
		// If returned in a response, indicates there are no more results.
		return ""
	}
	return pagination.Token(
		fmt.Sprintf("%d", t.UIPriority),
		string(t.ID.RootInvocationShardID.RootInvocationID),
		fmt.Sprintf("%d", t.ID.RootInvocationShardID.ShardIndex),
		t.ID.ModuleName,
		t.ID.ModuleScheme,
		t.ID.ModuleVariantHash,
		t.ID.CoarseName,
		t.ID.FineName,
		t.ID.CaseName,
		fmt.Sprintf("%d", t.RequestOrdinal),
	)
}

// makePageTokenForSummary creates a page token for the given TestVerdictSummary.
func makePageTokenForSummary(last *TestVerdictSummary) PageToken {
	if last.UIPriority != uiPriority(last.Status, last.StatusOverride) {
		// This is a tripwire to ensure that the SQL implementation of uiPriority
		// always matches the go implementation.
		panic("logic bug: SQL UI Priority derivation does not match go implementation")
	}
	// Encode all parts possibly relevant to ordering. whereAfterPageToken will use only
	// the elements it needs based on the ordering selected.
	return PageToken{
		UIPriority:     last.UIPriority,
		ID:             last.ID,
		RequestOrdinal: 0, // Summaries query does not support fetching nominated verdicts.
	}
}

// makePageToken creates a page token for the given TestVerdict.
func makePageToken(last *TestVerdict) PageToken {
	return PageToken{
		UIPriority:     uiPriority(last.Status, last.StatusOverride),
		ID:             last.ID,
		RequestOrdinal: last.RequestOrdinal,
	}
}

// uiPriority returns the UI priority of the given verdict.
func uiPriority(status pb.TestVerdict_Status, statusOverride pb.TestVerdict_StatusOverride) int64 {
	// Important: This logic must be kept in sync with the SQL Implementation
	// in query.go.
	if statusOverride == pb.TestVerdict_EXONERATED {
		return 90
	}
	switch status {
	case pb.TestVerdict_FAILED:
		// Highest priority.
		return 0
	case pb.TestVerdict_PRECLUDED:
		return 30
	case pb.TestVerdict_EXECUTION_ERRORED:
		return 30
	case pb.TestVerdict_FLAKY:
		return 70
	default:
		// Passed or skipped. Lowest priority.
		return MaxUIPriority
	}
}

// toTestResultsPageToken converts a test verdict page token to
// a test result page token.
func (t PageToken) toTestResultsPageToken() testresultsv2.PageToken {
	if t == (PageToken{}) {
		return testresultsv2.PageToken{}
	}
	var result testresultsv2.PageToken
	if t.RequestOrdinal > 0 {
		result.RequestOrdinal = t.RequestOrdinal
	} else {
		result.ID.RootInvocationShardID = t.ID.RootInvocationShardID
		result.ID.ModuleName = t.ID.ModuleName
		result.ID.ModuleScheme = t.ID.ModuleScheme
		result.ID.ModuleVariantHash = t.ID.ModuleVariantHash
		result.ID.CoarseName = t.ID.CoarseName
		result.ID.FineName = t.ID.FineName
		result.ID.CaseName = t.ID.CaseName
	}
	// We want to start *after* the last verdict.
	// U+10FFFF is the maximum unicode character and will sort after
	// all valid WorkUnits. It is not valid for work unit IDs itself.
	result.ID.WorkUnitID = "\U0010FFFF"
	result.ID.ResultID = ""
	return result
}

// toTestExonerationsPageToken converts a test verdict page token to
// a test exonerations page token.
func (t PageToken) toTestExonerationsPageToken() testexonerationsv2.PageToken {
	if t == (PageToken{}) {
		return testexonerationsv2.PageToken{}
	}
	var result testexonerationsv2.PageToken
	if t.RequestOrdinal > 0 {
		result.RequestOrdinal = t.RequestOrdinal
	} else {
		result.ID.RootInvocationShardID = t.ID.RootInvocationShardID
		result.ID.ModuleName = t.ID.ModuleName
		result.ID.ModuleScheme = t.ID.ModuleScheme
		result.ID.ModuleVariantHash = t.ID.ModuleVariantHash
		result.ID.CoarseName = t.ID.CoarseName
		result.ID.FineName = t.ID.FineName
		result.ID.CaseName = t.ID.CaseName
	}
	// We want to start *after* the last verdict.
	// U+10FFFF is the maximum unicode character and will sort after
	// all valid WorkUnits. It is not valid for work unit IDs itself.
	result.ID.WorkUnitID = "\U0010FFFF"
	result.ID.ExonerationID = ""
	return result
}
