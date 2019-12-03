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
)

// InclusionKey returns a spanner key for an Inclusion row.
func InclusionKey(including, included InvocationID) spanner.Key {
	return spanner.Key{including.RowID(), included.RowID()}
}

// ReadIncludedInvocations reads ids of included invocations.
func ReadIncludedInvocations(ctx context.Context, txn Txn, id InvocationID) (InvocationIDSet, error) {
	var ret InvocationIDSet
	var b Buffer
	err := txn.Read(ctx, "IncludedInvocations", id.Key().AsPrefix(), []string{"IncludedInvocationId"}).Do(func(row *spanner.Row) error {
		var included InvocationID
		if err := b.FromSpanner(row, &included); err != nil {
			return err
		}
		if ret == nil {
			ret = make(InvocationIDSet)
		}
		ret.Add(included)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}
