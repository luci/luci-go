// Copyright 2022 The LUCI Authors.
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
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	ctlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/pbutil"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
)

// JoinStatsHours is the number of previous hours
// ReadPresubmitRunJoinStatistics/ReadBuildJoinStatistics reads statistics for.
const JoinStatsHours = 36

// Entry is an ingestion control record, used to de-duplicate build ingestions
// and synchronise them with presubmit results (if required).
type Entry struct {
	// The identity of the build which is being ingested.
	// The scheme is: {buildbucket host name}/{build id}.
	BuildID string

	// Project is the LUCI Project the build belongs to. Used for
	// metrics monitoring join performance.
	BuildProject string

	// BuildResult is the result of the build bucket build, to be passed
	// to the result ingestion task. This is nil if the result is
	// not yet known.
	BuildResult *ctlpb.BuildResult

	// BuildJoinedTime is the Spanner commit time the build result was
	// populated. If the result has not yet been populated, this is the zero time.
	BuildJoinedTime time.Time

	// HasInvocation records wether the build has an associated (ResultDB)
	// invocation.
	// Value only populated once either BuildResult or InvocationResult populated.
	HasInvocation bool

	// Project is the LUCI Project the invocation belongs to. Used for
	// metrics monitoring join performance.
	InvocationProject string

	// InvocationResult is the result of the invocation, to be passed
	// to the result ingestion task. This is nil if the result is
	// not yet known.
	InvocationResult *ctlpb.InvocationResult

	// InvocationJoinedTime is the Spanner commit time the invocation result
	// was populated. If the result has not yet been populated, this is the zero time.
	InvocationJoinedTime time.Time

	// IsPresubmit records whether the build is part of a presubmit run.
	// If true, ingestion should wait for the presubmit result to be
	// populated (in addition to the build result) before commencing
	// ingestion.
	// Value only populated once either BuildResult or PresubmitResult populated.
	IsPresubmit bool

	// PresubmitProject is the LUCI Project the presubmit run belongs to.
	// This may differ from the LUCI Project teh build belongs to. Used for
	// metrics monitoring join performance.
	PresubmitProject string

	// PresubmitResult is result of the presubmit run, to be passed to the
	// result ingestion task. This is nil if the result is not yet known.
	PresubmitResult *ctlpb.PresubmitResult

	// PresubmitJoinedTime is the Spanner commit time the presubmit result was
	// populated. If the result has not yet been populated, this is the zero time.
	PresubmitJoinedTime time.Time

	// LastUpdated is the Spanner commit time the row was last updated.
	LastUpdated time.Time
}

// BuildID returns the control record key for a buildbucket build with the
// given hostname and ID.
func BuildID(hostname string, id int64) string {
	return fmt.Sprintf("%s/%v", hostname, id)
}

// Read reads ingestion control records for the specified build IDs.
// Exactly one *Entry is returned for each build ID. The result entry
// at index i corresponds to the buildIDs[i].
// If a record does not exist for the given build ID, an *Entry of
// nil is returned for that build ID.
func Read(ctx context.Context, buildIDs []string) ([]*Entry, error) {
	uniqueIDs := make(map[string]struct{})
	var keys []spanner.Key
	for _, buildID := range buildIDs {
		keys = append(keys, spanner.Key{buildID})
		if _, ok := uniqueIDs[buildID]; ok {
			return nil, fmt.Errorf("duplicate build ID %s", buildID)
		}
		uniqueIDs[buildID] = struct{}{}
	}
	cols := []string{
		"BuildID",
		"BuildProject",
		"BuildResult",
		"BuildJoinedTime",
		"HasInvocation",
		"InvocationProject",
		"InvocationResult",
		"InvocationJoinedTime",
		"IsPresubmit",
		"PresubmitProject",
		"PresubmitResult",
		"PresubmitJoinedTime",
		"LastUpdated",
	}
	entryByBuildID := make(map[string]*Entry)
	rows := span.Read(ctx, "Ingestions", spanner.KeySetFromKeys(keys...), cols)
	f := func(r *spanner.Row) error {
		var buildID string
		var buildProject spanner.NullString
		var buildResultBytes []byte
		var buildJoinedTime spanner.NullTime
		var hasInvocation spanner.NullBool
		var invocationProject spanner.NullString
		var invocationResultBytes []byte
		var invocationJoinedTime spanner.NullTime
		var isPresubmit spanner.NullBool
		var presubmitProject spanner.NullString
		var presubmitResultBytes []byte
		var presubmitJoinedTime spanner.NullTime
		var lastUpdated time.Time

		err := r.Columns(
			&buildID,
			&buildProject,
			&buildResultBytes,
			&buildJoinedTime,
			&hasInvocation,
			&invocationProject,
			&invocationResultBytes,
			&invocationJoinedTime,
			&isPresubmit,
			&presubmitProject,
			&presubmitResultBytes,
			&presubmitJoinedTime,
			&lastUpdated)
		if err != nil {
			return errors.Annotate(err, "read Ingestions row").Err()
		}
		var buildResult *ctlpb.BuildResult
		if buildResultBytes != nil {
			buildResult = &ctlpb.BuildResult{}
			if err := proto.Unmarshal(buildResultBytes, buildResult); err != nil {
				return errors.Annotate(err, "unmarshal build result").Err()
			}
		}
		var invocationResult *ctlpb.InvocationResult
		if invocationResultBytes != nil {
			invocationResult = &ctlpb.InvocationResult{}
			if err := proto.Unmarshal(invocationResultBytes, invocationResult); err != nil {
				return errors.Annotate(err, "unmarshal invocation result").Err()
			}
		}
		var presubmitResult *ctlpb.PresubmitResult
		if presubmitResultBytes != nil {
			presubmitResult = &ctlpb.PresubmitResult{}
			if err := proto.Unmarshal(presubmitResultBytes, presubmitResult); err != nil {
				return errors.Annotate(err, "unmarshal presubmit result").Err()
			}
		}

		entryByBuildID[buildID] = &Entry{
			BuildID:         buildID,
			BuildProject:    buildProject.StringVal,
			BuildResult:     buildResult,
			BuildJoinedTime: buildJoinedTime.Time,
			// HasInvocation uses NULL to indicate false.
			HasInvocation:        hasInvocation.Valid && hasInvocation.Bool,
			InvocationProject:    invocationProject.StringVal,
			InvocationResult:     invocationResult,
			InvocationJoinedTime: invocationJoinedTime.Time,
			// IsPresubmit uses NULL to indicate false.
			IsPresubmit:         isPresubmit.Valid && isPresubmit.Bool,
			PresubmitProject:    presubmitProject.StringVal,
			PresubmitResult:     presubmitResult,
			PresubmitJoinedTime: presubmitJoinedTime.Time,
			LastUpdated:         lastUpdated,
		}
		return nil
	}

	if err := rows.Do(f); err != nil {
		return nil, err
	}

	var result []*Entry
	for _, buildID := range buildIDs {
		// If the entry does not exist, return nil for that build ID.
		entry := entryByBuildID[buildID]
		result = append(result, entry)
	}
	return result, nil
}

// InsertOrUpdate creates or updates the given ingestion record.
// This operation is not safe to perform blindly; perform only in a
// read/write transaction with an attempted read of the corresponding entry.
func InsertOrUpdate(ctx context.Context, e *Entry) error {
	if err := validateEntry(e); err != nil {
		return err
	}
	update := map[string]any{
		"BuildId":              e.BuildID,
		"BuildProject":         spanner.NullString{Valid: e.BuildProject != "", StringVal: e.BuildProject},
		"BuildResult":          e.BuildResult,
		"BuildJoinedTime":      spanner.NullTime{Valid: e.BuildJoinedTime != time.Time{}, Time: e.BuildJoinedTime},
		"HasInvocation":        spanner.NullBool{Valid: e.HasInvocation, Bool: e.HasInvocation},
		"InvocationProject":    spanner.NullString{Valid: e.InvocationProject != "", StringVal: e.InvocationProject},
		"InvocationResult":     e.InvocationResult,
		"InvocationJoinedTime": spanner.NullTime{Valid: e.InvocationJoinedTime != time.Time{}, Time: e.InvocationJoinedTime},
		"IsPresubmit":          spanner.NullBool{Valid: e.IsPresubmit, Bool: e.IsPresubmit},
		"PresubmitProject":     spanner.NullString{Valid: e.PresubmitProject != "", StringVal: e.PresubmitProject},
		"PresubmitResult":      e.PresubmitResult,
		"PresubmitJoinedTime":  spanner.NullTime{Valid: e.PresubmitJoinedTime != time.Time{}, Time: e.PresubmitJoinedTime},
		"LastUpdated":          spanner.CommitTimestamp,
	}
	m := spanutil.InsertOrUpdateMap("Ingestions", update)
	span.BufferWrite(ctx, m)
	return nil
}

// JoinStatistics captures indicators of how well two join inputs
// (e.g. buildbucket build completions and presubmit run completions,
// or buildbucket build completions and invocation finalizations)
// are being joined.
type JoinStatistics struct {
	// TotalByHour captures the number of builds in the ingestions
	// table eligible to be joined (i.e. have the left-hand join input).
	//
	// Data is broken down by by hours since the build became
	// eligible for joining. Index 0 indicates the period
	// from ]-1 hour, now], index 1 indicates [-2 hour, -1 hour] and so on.
	TotalByHour []int64

	// JoinedByHour captures the number of builds in the ingestions
	// table eligible to be joined, which were successfully joined (have
	// results for both join inputs present).
	//
	// Data is broken down by by hours since the build became
	// eligible for joining. Index 0 indicates the period
	// from ]-1 hour, now], index 1 indicates [-2 hour, -1 hour] and so on.
	JoinedByHour []int64
}

// ReadBuildToPresubmitRunJoinStatistics measures the performance joining
// builds to presubmit runs.
//
// The statistics returned uses completed builds with a presubmit run
// as the denominator for measuring join performance.
// The performance joining to presubmit run results is then measured.
// Data is broken down by the project of the buildbucket build.
// The last 36 hours of data for each project is returned. Hours are
// measured since the buildbucket build result was received.
func ReadBuildToPresubmitRunJoinStatistics(ctx context.Context) (map[string]JoinStatistics, error) {
	stmt := spanner.NewStatement(`
		SELECT
		  BuildProject as project,
		  TIMESTAMP_DIFF(CURRENT_TIMESTAMP(), BuildJoinedTime, HOUR) as hour,
		  COUNT(*) as total,
		  COUNTIF(PresubmitResult IS NOT NULL) as joined,
		FROM Ingestions
		WHERE IsPresubmit
		  AND BuildJoinedTime >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL @hours HOUR)
		GROUP BY project, hour
	`)
	stmt.Params["hours"] = JoinStatsHours
	return readJoinStatistics(ctx, stmt)
}

// ReadPresubmitToBuildJoinStatistics measures the performance joining
// presubmit runs to builds.
//
// The statistics returned uses builds as reported by completed
// presubmit runs as the denominator for measuring join performance.
// The performance joining to buildbucket build results is then measured.
// Data is broken down by the project of the presubmit run.
// The last 36 hours of data for each project is returned. Hours are
// measured since the presubmit run result was received.
func ReadPresubmitToBuildJoinStatistics(ctx context.Context) (map[string]JoinStatistics, error) {
	stmt := spanner.NewStatement(`
		SELECT
		  PresubmitProject as project,
		  TIMESTAMP_DIFF(CURRENT_TIMESTAMP(), PresubmitJoinedTime, HOUR) as hour,
		  COUNT(*) as total,
		  COUNTIF(BuildResult IS NOT NULL) as joined,
		FROM Ingestions
		WHERE IsPresubmit
		  AND PresubmitJoinedTime >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL @hours HOUR)
		GROUP BY project, hour
	`)
	stmt.Params["hours"] = JoinStatsHours
	return readJoinStatistics(ctx, stmt)
}

// ReadBuildToInvocationJoinStatistics measures the performance joining
// builds to finalized invocations.
//
// The statistics returned uses completed builds with an invocation
// as the denominator for measuring join performance.
// The performance joining to finalized invocations is then measured.
// Data is broken down by the project of the buildbucket build.
// The last 36 hours of data for each project is returned. Hours are
// measured since the buildbucket build result was received.
func ReadBuildToInvocationJoinStatistics(ctx context.Context) (map[string]JoinStatistics, error) {
	stmt := spanner.NewStatement(`
		SELECT
		  BuildProject as project,
		  TIMESTAMP_DIFF(CURRENT_TIMESTAMP(), BuildJoinedTime, HOUR) as hour,
		  COUNT(*) as total,
		  COUNTIF(InvocationResult IS NOT NULL) as joined,
		FROM Ingestions
		WHERE HasInvocation
		  AND BuildJoinedTime >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL @hours HOUR)
		GROUP BY project, hour
	`)
	stmt.Params["hours"] = JoinStatsHours
	return readJoinStatistics(ctx, stmt)
}

// ReadInvocationToBuildJoinStatistics measures the performance joining
// finalized invocations to builds.
//
// The statistics returned uses finalized invocations (for buildbucket builds)
// as the denominator for measuring join performance.
// The performance joining to buildbucket build results is then measured.
// Data is broken down by the project of the ingested invocation (this
// should be the same as the ingested build, although it comes from a
// different source).
// The last 36 hours of data for each project is returned. Hours are
// measured since the finalized invocation was received.
func ReadInvocationToBuildJoinStatistics(ctx context.Context) (map[string]JoinStatistics, error) {
	stmt := spanner.NewStatement(`
		SELECT
		  InvocationProject as project,
		  TIMESTAMP_DIFF(CURRENT_TIMESTAMP(), InvocationJoinedTime, HOUR) as hour,
		  COUNT(*) as total,
		  COUNTIF(BuildResult IS NOT NULL) as joined,
		FROM Ingestions
		WHERE HasInvocation
		  AND InvocationJoinedTime >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL @hours HOUR)
		GROUP BY project, hour
	`)
	stmt.Params["hours"] = JoinStatsHours
	return readJoinStatistics(ctx, stmt)
}

func readJoinStatistics(ctx context.Context, stmt spanner.Statement) (map[string]JoinStatistics, error) {
	result := make(map[string]JoinStatistics)
	it := span.Query(ctx, stmt)
	err := it.Do(func(r *spanner.Row) error {
		var project string
		var hour int64
		var total, joined int64

		err := r.Columns(&project, &hour, &total, &joined)
		if err != nil {
			return errors.Annotate(err, "read row").Err()
		}

		stats, ok := result[project]
		if !ok {
			stats = JoinStatistics{
				// Add zero data for all hours.
				TotalByHour:  make([]int64, JoinStatsHours),
				JoinedByHour: make([]int64, JoinStatsHours),
			}
		}
		stats.TotalByHour[hour] = total
		stats.JoinedByHour[hour] = joined

		result[project] = stats
		return nil
	})
	if err != nil {
		return nil, errors.Annotate(err, "query presubmit join stats by project").Err()
	}
	return result, nil
}

func validateEntry(e *Entry) error {
	if e.BuildID == "" {
		return errors.New("build ID must be specified")
	}

	if e.BuildResult != nil {
		if err := ValidateBuildResult(e.BuildResult); err != nil {
			return errors.Annotate(err, "build result").Err()
		}
		if err := pbutil.ValidateProject(e.BuildProject); err != nil {
			return errors.Annotate(err, "build project").Err()
		}
	} else {
		if e.BuildProject != "" {
			return errors.New("build project must only be specified" +
				" if build result is specified")
		}
	}

	if e.InvocationResult != nil {
		if !e.HasInvocation {
			return errors.New("invocation result must not be set unless HasInvocation is set")
		}
		if err := pbutil.ValidateProject(e.InvocationProject); err != nil {
			return errors.Annotate(err, "invocation project").Err()
		}
	} else {
		if e.InvocationProject != "" {
			return errors.New("invocation project must only be specified" +
				" if invocation result is specified")
		}
	}

	if e.PresubmitResult != nil {
		if !e.IsPresubmit {
			return errors.New("presubmit result must not be set unless IsPresubmit is set")
		}
		if err := ValidatePresubmitResult(e.PresubmitResult); err != nil {
			return errors.Annotate(err, "presubmit result").Err()
		}
		if err := pbutil.ValidateProject(e.PresubmitProject); err != nil {
			return errors.Annotate(err, "presubmit project").Err()
		}
	} else {
		if e.PresubmitProject != "" {
			return errors.New("presubmit project must only be specified" +
				" if presubmit result is specified")
		}
	}

	return nil
}

func ValidateBuildResult(r *ctlpb.BuildResult) error {
	switch {
	case r.Host == "":
		return errors.New("host must be specified")
	case r.Id == 0:
		return errors.New("id must be specified")
	case !r.CreationTime.IsValid():
		return errors.New("creation time must be specified")
	case r.Project == "":
		return errors.New("project must be specified")
	case r.HasInvocation && r.ResultdbHost == "":
		return errors.New("resultdb_host must be specified if has_invocation set")
	case r.Builder == "":
		return errors.New("builder must be specified")
	case r.Status == analysispb.BuildStatus_BUILD_STATUS_UNSPECIFIED:
		return errors.New("build status must be specified")
	}
	return nil
}

func ValidatePresubmitResult(r *ctlpb.PresubmitResult) error {
	switch {
	case r.PresubmitRunId == nil:
		return errors.New("presubmit run ID must be specified")
	case r.PresubmitRunId.System != "luci-cv":
		// LUCI CV is currently the only supported system.
		return errors.New("presubmit run system must be 'luci-cv'")
	case r.PresubmitRunId.Id == "":
		return errors.New("presubmit run system-specific ID must be specified")
	case !r.CreationTime.IsValid():
		return errors.New("creation time must be specified and valid")
	case r.Status == analysispb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_UNSPECIFIED:
		return errors.New("status must be specified")
	case r.Mode == analysispb.PresubmitRunMode_PRESUBMIT_RUN_MODE_UNSPECIFIED:
		return errors.New("mode must be specified")
	}
	return nil
}
