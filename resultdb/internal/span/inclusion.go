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

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// InclusionKey returns a spanner key for an Inclusion row.
func InclusionKey(including, included InvocationID) spanner.Key {
	return spanner.Key{including.RowID(), included.RowID()}
}

// ReadInclusions reads all inclusions, if any, of an invocation within the transaction.
func ReadInclusions(ctx context.Context, txn Txn, id InvocationID) (map[string]*pb.Invocation_InclusionAttrs, error) {
	ret := map[string]*pb.Invocation_InclusionAttrs{}
	err := txn.Read(ctx, "Inclusions", id.Key().AsPrefix(), []string{"IncludedInvocationId"}).Do(func(row *spanner.Row) error {
		var included InvocationID
		if err := FromSpanner(row, &included); err != nil {
			return err
		}
		ret[included.Name()] = &pb.Invocation_InclusionAttrs{}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}
