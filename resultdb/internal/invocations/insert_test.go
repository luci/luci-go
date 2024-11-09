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
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func insertInvocation(id ID, extraValues map[string]any) *spanner.Mutation {
	future := time.Date(2050, 1, 1, 0, 0, 0, 0, time.UTC)
	values := map[string]any{
		"InvocationId":                      id,
		"ShardId":                           0,
		"State":                             pb.Invocation_FINALIZED,
		"Realm":                             "",
		"InvocationExpirationTime":          future,
		"ExpectedTestResultsExpirationTime": future,
		"CreateTime":                        spanner.CommitTimestamp,
		"Deadline":                          future,
		"Submitted":                         false,
	}
	updateDict(values, extraValues)

	// Ensure a finalized invocation has finalization time.
	if _, ok := values["FinalizeTime"]; !ok && values["State"].(pb.Invocation_State) == pb.Invocation_FINALIZED {
		values["FinalizeTime"] = spanner.CommitTimestamp
	}

	return spanutil.InsertMap("Invocations", values)
}

func insertInclusion(including, included ID) *spanner.Mutation {
	return spanutil.InsertMap("IncludedInvocations", map[string]any{
		"InvocationId":         including,
		"IncludedInvocationId": included,
	})
}

func insertInvocationIncluding(id ID, included ...ID) []*spanner.Mutation {
	ms := []*spanner.Mutation{insertInvocation(id, nil)}
	for _, incl := range included {
		ms = append(ms, insertInclusion(id, incl))
	}
	return ms
}

func updateDict(dest, source map[string]any) {
	for k, v := range source {
		dest[k] = v
	}
}
