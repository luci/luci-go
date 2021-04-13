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

package invocations

import (
	"bytes"
	"context"
	"regexp"
	"text/template"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	year = 365 * 24 * time.Hour

	// HistoryWindow specifies how far back to query results history by
	// default.
	HistoryWindow = 2 * year

	// ClockDriftBuffer how much time to add to the current time when
	// using it as a default upper bound for the query window.
	ClockDriftBuffer = 5 * time.Minute
)

// HistoryQuery specifies invocations to fetch to retrieve test result history.
type HistoryQuery struct {
	Realm     string
	TimeRange *pb.TimeRange
	Predicate *pb.TestResultPredicate
}

// ByTimestamp queries indexed invocations in a given time range.
// It executes the callback once for each row, starting with the most recent.
func (q *HistoryQuery) ByTimestamp(ctx context.Context, callback func(inv ID, ts *timestamp.Timestamp) error) error {
	var err error
	now := clock.Now(ctx)

	// We keep results for up to ~2 years, use this lower bound if one is not
	// given.
	minTime := now.Add(-HistoryWindow)
	if q.TimeRange.GetEarliest() != nil {
		if minTime, err = ptypes.Timestamp(q.TimeRange.GetEarliest()); err != nil {
			return errors.Annotate(err, "timeRange.earliest").Err()
		}
	}

	// If unspecified, get results up to the present time.
	// Plus a buffer to account for possible clock drift.
	maxTime := now.Add(ClockDriftBuffer)
	if q.TimeRange.GetLatest() != nil {
		if maxTime, err = ptypes.Timestamp(q.TimeRange.GetLatest()); err != nil {
			return errors.Annotate(err, "timeRange.latest").Err()
		}
	}

	literalPrefix := ""
	if re := q.Predicate.GetTestIdRegexp(); re != "" && re != ".*" {
		r := regexp.MustCompile(re)
		// We're trying to match the invocation's CommonTestIDPrefix with re,
		// so re should have a literal prefix, otherwise the match likely
		// fails.
		// For example if an invocation's CommonTestIDPrefix is "ninja://" and
		// re is ".*browser_tests.*", we wouldn't know if that invocation contains
		// any test results with matching test ids or not.
		literalPrefix, _ = r.LiteralPrefix()
	}

	sql := &bytes.Buffer{}
	if err = queryTmpl.Execute(sql, map[string]interface{}{"MatchTestIdPrefix": literalPrefix != ""}); err != nil {
		return err
	}
	st := spanner.NewStatement(sql.String())
	st.Params["realm"] = q.Realm
	st.Params["minTime"] = minTime
	st.Params["maxTime"] = maxTime
	// Use literalPrefix instead of q.Predicate.GetTestIdRegexp() to match all
	// possible invocations.
	// For example if an invocation's CommonTestIDPrefix is "ninja://chrome/test:browser_tests" and
	// q.Predicate.GetTestIdRegexp() is "ninja://.*browser_tests.*", using literalPrefix
	// would guarantee that this invocation will be included in the query results.
	st.Params["literalPrefix"] = literalPrefix

	var b spanutil.Buffer
	return spanutil.Query(ctx, st, func(row *spanner.Row) error {
		var inv ID
		var ts *timestamp.Timestamp
		if err := b.FromSpanner(row, &inv, &ts); err != nil {
			return err
		}
		return callback(inv, ts)
	})
}

var queryTmpl = template.Must(template.New("").Parse(`
	SELECT
		i.InvocationId,
		i.HistoryTime,
	FROM Invocations@{FORCE_INDEX=InvocationsByTimestamp, spanner_emulator.disable_query_null_filtered_index_check=true} i
	WHERE i.Realm = @realm AND i.HistoryTime BETWEEN @minTime AND @maxTime
	{{if .MatchTestIdPrefix}}
		AND (
			-- For the cases where literalPrefix is long and specific, for example:
			-- literalPrefix = "ninja://chrome/test:browser_tests/AutomationApiTest"
			-- i.CommonTestIDPrefix = "ninja://chrome/test:browser_tests/"
			STARTS_WITH(@literalPrefix, IFNULL(i.CommonTestIDPrefix, "")) OR
			-- For the cases where literalPrefix is short, likely because q.Predicate.TestIdRegexp
			-- contains non-literal regexp, for example:
			-- q.Predicate.TestIdRegexp = "ninja://.*browser_tests/" which makes literalPrefix
			-- to be "ninja://".
			-- This condition is not very useful to improve the performance of test
			-- result history API, but without it will make the results incomplete.
			STARTS_WITH(i.CommonTestIDPrefix, @literalPrefix)
		)
	{{end}}
	ORDER BY i.HistoryTime DESC
`))
