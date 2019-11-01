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

package span

import (
	"context"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// ReadInclusions reads all inclusions, if any, of an invocation within the transaction.
func ReadInclusions(ctx context.Context, txn Txn, invID string) (map[string]*pb.Invocation_InclusionAttrs, error) {
	st := spanner.NewStatement(`
		SELECT
			incl.IncludedInvocationId,
			incl.OverriddenByIncludedInvocationId,
			IFNULL(included.FinalizeTime < including.FinalizeTime, included.FinalizeTime IS NOT NULL) as ready
		FROM Invocations including
		JOIN Inclusions incl ON including.InvocationId = incl.InvocationId
		JOIN Invocations included ON incl.IncludedInvocationId = included.InvocationId
		WHERE including.InvocationId = @invID
	`)
	st.Params["invID"] = invID
	it := txn.Query(ctx, st)
	defer it.Stop()

	inclusions := map[string]*pb.Invocation_InclusionAttrs{}
	for {
		row, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		var included, overriddenByID string
		attr := &pb.Invocation_InclusionAttrs{}
		if err := FromSpanner(row, &included, &overriddenByID, &attr.Ready); err != nil {
			return nil, err
		}
		if overriddenByID != "" {
			attr.OverriddenBy = pbutil.InvocationName(overriddenByID)
		}
		inclusions[pbutil.InvocationName(included)] = attr
	}

	return inclusions, nil
}
