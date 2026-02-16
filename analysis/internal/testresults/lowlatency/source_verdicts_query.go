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

package lowlatency

import (
	"context"
	"text/template"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/data/aip160"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/testresults"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// SourceVerdictV2 represents an aggregation of all test results at a source position.
type SourceVerdictV2 struct {
	// The source position.
	Position int64
	// The overall status of the test at this position.
	// This status is computed using submitted sources only.
	Status pb.TestVerdict_Status
	// The approximate status of the test at this position.
	// This status is computed using data from presubmit, so is set more often
	// than the above, but is also more noisy. Use with judgement.
	ApproximateStatus pb.TestVerdict_Status
	// Test verdicts at the position. Limited to 20.
	InvocationVerdicts []InvocationTestVerdictV2
}

// InvocationTestVerdictV2 is a test verdict that is part of a source verdict.
type InvocationTestVerdictV2 struct {
	// The root invocation ID for the verdict.
	// Either this or InvocationID will be set.
	RootInvocationID string
	// The invocation for the verdict.
	InvocationID string
	// Partition time of the test verdict.
	PartitionTime time.Time
	// Status of the test verdict.
	Status pb.TestVerdict_Status
	// Changelists tested by the verdict.
	// If this is non-empty, the verdict does not contribute to `status`.
	Changelists []testresults.Changelist
	// Whether the sources used by the verdict were dirty.
	// Such verdicts does not contribute to either `status` or `approximate_status`.
	HasDirtySources bool
}

// SourceVerdictQuery queries source verdicts for a (test, source ref).
type SourceVerdictQuery struct {
	// The LUCI Project. Required.
	Project string
	// The Test ID. Required.
	TestID string
	// The test variant hash. Required.
	VariantHash string
	// The source ref hash. Required.
	RefHash []byte
	// Only test verdicts with allowed invocation realms can be returned.
	AllowedSubrealms []string
	// The AIP-160 filter to apply to the returned source verdicts.
	Filter string
	// The maximum number of source verdicts to return.
	PageSize int
}

// sourceVerdictsFilterSchema defines the AIP-160 filter schema for QuerySourceVerdicts.
var sourceVerdictsFilterSchema = aip160.NewDatabaseTable().WithFields(
	aip160.NewField().WithFieldPath("position").WithBackend(aip160.NewIntegerColumn("SourcePosition")).Filterable().Build(),
).Build()

// ValidateSourceVerdictsFilter validates an AIP-160 filter string for QuerySourceVerdicts.
func ValidateSourceVerdictsFilter(filter string) error {
	if filter == "" {
		return nil
	}
	// Max length check could be added here if needed, similar to ResultDB.
	f, err := aip160.ParseFilter(filter)
	if err != nil {
		return err
	}
	_, _, err = sourceVerdictsFilterSchema.WhereClause(f, "", "")
	return err
}

type PageToken struct {
	// The last source position returned.
	LastSourcePosition int64
}

// Fetch reads source verdicts matching the query.
// Must be called in a spanner transactional context.
func (q *SourceVerdictQuery) Fetch(ctx context.Context, pageToken PageToken) ([]SourceVerdictV2, PageToken, error) {
	st, err := q.buildQuery(pageToken)
	if err != nil {
		return nil, PageToken{}, err
	}

	it := span.Query(ctx, st)
	var verdicts []SourceVerdictV2
	err = it.Do(func(r *spanner.Row) error {
		var v SourceVerdictV2
		if err := r.ColumnByName("SourcePosition", &v.Position); err != nil {
			return errors.Fmt("read SourcePosition: %w", err)
		}
		if err := r.ColumnByName("Status", &v.Status); err != nil {
			return errors.Fmt("read Status: %w", err)
		}
		if err := r.ColumnByName("ApproximateStatus", &v.ApproximateStatus); err != nil {
			return errors.Fmt("read ApproximateStatus: %w", err)
		}

		var invVerdicts []*struct {
			RootInvocationID     string
			Status               int64
			PartitionTime        time.Time
			ChangelistHosts      []string
			ChangelistChanges    []int64
			ChangelistPatchsets  []int64
			ChangelistOwnerKinds []string
			HasDirtySources      bool
		}
		if err := r.ColumnByName("InvocationVerdicts", &invVerdicts); err != nil {
			return errors.Fmt("read InvocationVerdicts: %w", err)
		}

		v.InvocationVerdicts = make([]InvocationTestVerdictV2, 0, len(invVerdicts))
		for _, iv := range invVerdicts {
			cls := make([]testresults.Changelist, len(iv.ChangelistHosts))
			for i := range iv.ChangelistHosts {
				cls[i] = testresults.Changelist{
					Host:      testresults.DecompressHost(iv.ChangelistHosts[i]),
					Change:    iv.ChangelistChanges[i],
					Patchset:  iv.ChangelistPatchsets[i],
					OwnerKind: testresults.OwnerKindFromDB(iv.ChangelistOwnerKinds[i]),
				}
			}
			rootInvID := RootInvocationIDFromRowID(iv.RootInvocationID)
			resultItem := InvocationTestVerdictV2{
				PartitionTime:   iv.PartitionTime,
				Status:          pb.TestVerdict_Status(iv.Status),
				Changelists:     cls,
				HasDirtySources: iv.HasDirtySources,
			}
			if rootInvID.IsLegacy {
				resultItem.InvocationID = rootInvID.Value
			} else {
				resultItem.RootInvocationID = rootInvID.Value
			}
			v.InvocationVerdicts = append(v.InvocationVerdicts, resultItem)
		}
		verdicts = append(verdicts, v)
		return nil
	})
	if err != nil {
		return nil, PageToken{}, err
	}

	nextPageToken := PageToken{}
	if len(verdicts) == q.PageSize {
		last := verdicts[len(verdicts)-1]
		nextPageToken = PageToken{
			LastSourcePosition: last.Position,
		}
	}

	return verdicts, nextPageToken, nil
}

// buildQuery builds a Spanner query for the SourceVerdictQuery.
func (q *SourceVerdictQuery) buildQuery(pageToken PageToken) (spanner.Statement, error) {
	params := map[string]any{
		"project":          q.Project,
		"testID":           q.TestID,
		"variantHash":      q.VariantHash,
		"refHash":          q.RefHash,
		"allowedSubrealms": q.AllowedSubrealms,
		"limit":            q.PageSize,
	}

	paginationClause := "TRUE"
	if pageToken != (PageToken{}) {
		paginationClause = q.whereAfterPageToken(pageToken, params)
	}

	whereClause := "TRUE"
	if q.Filter != "" {
		f, err := aip160.ParseFilter(q.Filter)
		if err != nil {
			return spanner.Statement{}, errors.Fmt("parse filter: %v", err)
		}
		clause, additionalParams, err := sourceVerdictsFilterSchema.WhereClause(f, "", "p_")
		if err != nil {
			return spanner.Statement{}, errors.Fmt("generate where clause: %v", err)
		}
		for _, p := range additionalParams {
			// All parameters should be prefixed by "ctrf_" so should not conflict with existing parameters.
			params[p.Name] = p.Value
		}
		whereClause = "(" + clause + ")"
	}

	tmplInput := map[string]any{
		"FilterClause":             whereClause,
		"PaginationClause":         paginationClause,
		"ResultV2Passed":           int64(pb.TestResult_PASSED),
		"ResultV2Failed":           int64(pb.TestResult_FAILED),
		"ResultV2Skipped":          int64(pb.TestResult_SKIPPED),
		"ResultV2ExecutionErrored": int64(pb.TestResult_EXECUTION_ERRORED),
		"ResultV2Precluded":        int64(pb.TestResult_PRECLUDED),
		"ResultV1Skip":             int64(pb.TestResultStatus_SKIP),
		"VerdictPassed":            int64(pb.TestVerdict_PASSED),
		"VerdictFailed":            int64(pb.TestVerdict_FAILED),
		"VerdictSkipped":           int64(pb.TestVerdict_SKIPPED),
		"VerdictExecutionErrored":  int64(pb.TestVerdict_EXECUTION_ERRORED),
		"VerdictPrecluded":         int64(pb.TestVerdict_PRECLUDED),
		"VerdictFlaky":             int64(pb.TestVerdict_FLAKY),
		"VerdictUnspecified":       int64(pb.TestVerdict_STATUS_UNSPECIFIED),
	}

	st, err := spanutil.GenerateStatement(sourceVerdictQueryTmpl, sourceVerdictQueryTmpl.Name(), tmplInput)
	if err != nil {
		return spanner.Statement{}, err
	}
	st.Params = params
	return st, nil
}

// whereAfterPageToken returns a WHERE clause for the given page token.
func (q *SourceVerdictQuery) whereAfterPageToken(token PageToken, params map[string]any) string {
	params["lastSourcePosition"] = token.LastSourcePosition
	return "SourcePosition < @lastSourcePosition"
}

var sourceVerdictQueryTmpl = template.Must(template.New("").Parse(`
-- We do not use WITH clauses below as Spanner query optimizer does not optimize
-- across WITH clause/CTE boundaries and this results in suboptimal query plans. Instead
-- we use templates to include the nested SQL statements.
{{define "InvocationVerdicts"}}
		-- Invocation verdicts.
		SELECT
			SourcePosition,
			RootInvocationId,
			PassedCount,
			FailedCount,
			SkippedCount,
			ExecutionErroredCount,
			PrecludedCount,
			(CASE WHEN PassedCount > 0 AND FailedCount > 0 THEN {{.VerdictFlaky}}
						WHEN PassedCount > 0 THEN {{.VerdictPassed}}
						WHEN FailedCount > 0 THEN {{.VerdictFailed}}
						WHEN SkippedCount > 0 THEN {{.VerdictSkipped}}
						WHEN ExecutionErroredCount > 0 THEN {{.VerdictExecutionErrored}}
						ELSE {{.VerdictPrecluded}} END) AS Status,
			ARRAY_LENGTH(ChangelistChanges) = 0 AND NOT HasDirtySources AS IsSubmittedSources,
			NOT HasDirtySources AS IsSubmittedOrPresubmitSources,
			PartitionTime,
			ChangelistHosts,
			ChangelistChanges,
			ChangelistPatchsets,
			ChangelistOwnerKinds,
			HasDirtySources
		FROM (
			SELECT
				SourcePosition,
				RootInvocationId,
				COUNTIF(StatusV2 = {{.ResultV2Skipped}} OR (StatusV2 IS NULL AND Status = {{.ResultV1Skip}} AND NOT IsUnexpected)) AS SkippedCount,
				COUNTIF(StatusV2 = {{.ResultV2Passed}} OR (StatusV2 IS NULL AND Status <> {{.ResultV1Skip}} AND NOT IsUnexpected)) AS PassedCount,
				COUNTIF(StatusV2 = {{.ResultV2Failed}} OR (StatusV2 IS NULL AND Status <> {{.ResultV1Skip}} AND IsUnexpected)) AS FailedCount,
				COUNTIF(StatusV2 = {{.ResultV2ExecutionErrored}} OR (StatusV2 IS NULL AND Status = {{.ResultV1Skip}} AND IsUnexpected)) AS ExecutionErroredCount,
				COUNTIF(StatusV2 = {{.ResultV2Precluded}}) AS PrecludedCount,
				ANY_VALUE(PartitionTime) AS PartitionTime,
				ANY_VALUE(ChangelistHosts) as ChangelistHosts,
				ANY_VALUE(ChangelistChanges) as ChangelistChanges,
				ANY_VALUE(ChangelistPatchsets) as ChangelistPatchsets,
				ANY_VALUE(ChangelistOwnerKinds) as ChangelistOwnerKinds,
				ANY_VALUE(HasDirtySources) as HasDirtySources
			FROM TestResultsBySourcePosition
			WHERE Project = @project
				AND TestId = @testID
				AND VariantHash = @variantHash
				AND SourceRefHash = @refHash
				AND SubRealm IN UNNEST(@allowedSubrealms)
			GROUP BY SourcePosition, RootInvocationId
	)
{{end}}
{{define "SourceVerdicts"}}
	-- Source verdicts.
	SELECT
		SourcePosition,
		ARRAY_AGG(STRUCT(
			RootInvocationId,
			Status,
			PartitionTime,
			ChangelistHosts,
			ChangelistChanges,
			ChangelistPatchsets,
			ChangelistOwnerKinds,
			HasDirtySources
		)) as InvocationVerdicts,
		-- Status computed from verdicts with submitted sources only.
		(CASE WHEN SUM(IF(IsSubmittedSources, PassedCount, 0)) > 0
					AND SUM(IF(IsSubmittedSources, FailedCount, 0)) > 0 THEN {{.VerdictFlaky}}
				WHEN SUM(IF(IsSubmittedSources, PassedCount, 0)) > 0 THEN {{.VerdictPassed}}
				WHEN SUM(IF(IsSubmittedSources, FailedCount, 0)) > 0 THEN {{.VerdictFailed}}
				WHEN SUM(IF(IsSubmittedSources, SkippedCount, 0)) > 0 THEN {{.VerdictSkipped}}
				WHEN SUM(IF(IsSubmittedSources, ExecutionErroredCount, 0)) > 0 THEN {{.VerdictExecutionErrored}}
				WHEN COUNTIF(IsSubmittedSources) > 0 THEN {{.VerdictPrecluded}}
				ELSE {{.VerdictUnspecified}} END) AS Status,
		-- Approximate status is computed from presubmit and submitted verdicts.
		(CASE WHEN SUM(IF(IsSubmittedOrPresubmitSources, PassedCount, 0)) > 0 AND SUM(IF(IsSubmittedOrPresubmitSources, FailedCount, 0)) > 0 THEN {{.VerdictFlaky}}
				WHEN SUM(IF(IsSubmittedOrPresubmitSources, PassedCount, 0)) > 0 THEN {{.VerdictPassed}}
				WHEN SUM(IF(IsSubmittedOrPresubmitSources, FailedCount, 0)) > 0 THEN {{.VerdictFailed}}
				WHEN SUM(IF(IsSubmittedOrPresubmitSources, SkippedCount, 0)) > 0 THEN {{.VerdictSkipped}}
				WHEN SUM(IF(IsSubmittedOrPresubmitSources, ExecutionErroredCount, 0)) > 0 THEN {{.VerdictExecutionErrored}}
				WHEN COUNTIF(IsSubmittedOrPresubmitSources) > 0 THEN {{.VerdictPrecluded}}
				ELSE {{.VerdictUnspecified}} END) AS ApproximateStatus
	FROM (
		{{template "InvocationVerdicts" .}}
	)
	GROUP BY SourcePosition
{{end}}

SELECT
	SourcePosition,
	ARRAY(
		SELECT AS STRUCT R.*
		FROM UNNEST(InvocationVerdicts) R
		ORDER BY R.PartitionTime DESC, R.RootInvocationId
		LIMIT 20
	) AS InvocationVerdicts,
	Status,
	ApproximateStatus
FROM (
	{{template "SourceVerdicts" .}}
)
WHERE {{.FilterClause}}
	AND {{.PaginationClause}}
ORDER BY SourcePosition DESC
LIMIT @limit
`))
