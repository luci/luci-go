// Copyright 2021 The LUCI Authors.
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

	"cloud.google.com/go/spanner"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// InvsMatchingPredicate returns all invocations matching the predicate.
func InvsMatchingPredicate(ctx context.Context, invIDs IDSet, predicate *pb.TestResultPredicate) (IDSet, error) {
	if predicate == nil {
		return invIDs, nil
	}

	literalPrefix := ""
	if re := predicate.GetTestIdRegexp(); re != "" && re != ".*" {
		r := regexp.MustCompile(re)
		// We're trying to match the invocation's CommonTestIDPrefix with re,
		// so re should have a literal prefix, otherwise the match likely
		// fails.
		// For example if an invocation's CommonTestIDPrefix is "ninja://" and
		// re is ".*browser_tests.*", we wouldn't know if that invocation contains
		// any test results with matching test ids or not.
		literalPrefix, _ = r.LiteralPrefix()
	}

	var variant []string
	if predicate.GetVariant() != nil {
		switch p := predicate.GetVariant().GetPredicate().(type) {
		case *pb.VariantPredicate_Equals:
			variant = pbutil.VariantToStrings(p.Equals)
		case *pb.VariantPredicate_Contains:
			variant = pbutil.VariantToStrings(p.Contains)
		case nil:
		// No filter.
		default:
			panic(errors.Reason("unexpected variant predicate %q", predicate.GetVariant()).Err())
		}
	}

	if literalPrefix == "" && len(variant) == 0 {
		// Nothing to filter out.
		return invIDs, nil
	}

	ids := make(IDSet, len(invIDs))
	sql := &bytes.Buffer{}
	if err := queryTmpl.Execute(sql, map[string]interface{}{
		"MatchTestIdPrefix": literalPrefix != "",
		"MatchVariant":      len(variant) != 0,
	}); err != nil {
		return ids, err
	}
	st := spanner.NewStatement(sql.String())
	st.Params["invIDs"] = invIDs
	// Use literalPrefix instead of predicate.GetTestIdRegexp() to match all
	// possible invocations.
	// For example if an invocation's CommonTestIDPrefix is "ninja://chrome/test:browser_tests" and
	// predicate.GetTestIdRegexp() is "ninja://.*browser_tests.*", using literalPrefix
	// would guarantee that this invocation will be included in the query results.
	st.Params["literalPrefix"] = literalPrefix
	st.Params["variant"] = variant

	var b spanutil.Buffer
	err := spanutil.Query(ctx, st, func(row *spanner.Row) error {
		var inv ID
		if err := b.FromSpanner(row, &inv); err != nil {
			return err
		}
		ids.Add(inv)
		return nil
	})
	return ids, err
}

var queryTmpl = template.Must(template.New("").Parse(`
	SELECT
		i.InvocationId,
	FROM Invocations i
	WHERE i.InvocationId IN UNNEST(@invIDs)
	{{if .MatchTestIdPrefix}}
		AND i.CommonTestIDPrefix IS NOT NULL
		AND (
			-- For the cases where literalPrefix is long and specific, for example:
			-- literalPrefix = "ninja://chrome/test:browser_tests/AutomationApiTest"
			-- i.CommonTestIDPrefix = "ninja://chrome/test:browser_tests/"
			STARTS_WITH(@literalPrefix, i.CommonTestIDPrefix) OR
			-- For the cases where literalPrefix is short, likely because predicate.TestIdRegexp
			-- contains non-literal regexp, for example:
			-- predicate.TestIdRegexp = "ninja://.*browser_tests/" which makes literalPrefix
			-- to be "ninja://".
			-- This condition is not very useful to improve the performance of test
			-- result history API, but without it will make the results incomplete.
			STARTS_WITH(i.CommonTestIDPrefix, @literalPrefix)
		)
	{{end}}
	{{if .MatchVariant}}
		AND (SELECT LOGICAL_AND(kv IN UNNEST(TestResultVariantUnion)) FROM UNNEST(@variant) kv)
	{{end}}
	ORDER BY i.HistoryTime DESC
`))
