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
	"time"

	"cloud.google.com/go/spanner"
	tspb "github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// This file implements utility functions that make spanner API slightly easier
// to use.

// Txn is implemented by all spanner transactions.
type Txn interface {
	// ReadRow reads a single row from the database.
	ReadRow(ctx context.Context, table string, key spanner.Key, columns []string) (*spanner.Row, error)
}

func slices(m map[string]interface{}) (keys []string, values []interface{}) {
	keys = make([]string, 0, len(m))
	values = make([]interface{}, 0, len(m))
	for k, v := range m {
		keys = append(keys, k)
		values = append(values, v)
	}
	return
}

// ReadRow reads a single row from the database and reads its values.
// ptrMap must map from column names to pointers where the values will be
// written.
func ReadRow(ctx context.Context, txn Txn, table string, key spanner.Key, ptrMap map[string]interface{}) error {
	columns, ptrs := slices(ptrMap)
	row, err := txn.ReadRow(ctx, table, key, columns)
	if err != nil {
		return err
	}

	// Generate new pointer map with substitutions for Spanner-unsupported types.
	subInds := replacePointers(ptrs)

	if err := row.Columns(ptrs...); err != nil {
		return err
	}

	// Assign the all the values back into the original pointer map.
	return replaceValues(ptrs, subInds)
}

// replacePointers replaces the pointers to Spanner-unsupported types with
// ones Spanner does support and returns a mapping containing the old pointers.
func replacePointers(ptrs []interface{}) map[int]interface{} {
	// subInds maps the index of substituted type to the original pointer.
	subInds := map[int]interface{}{}

	for i, ptr := range ptrs {
		if _, ok := ptr.(**tspb.Timestamp); ok {
			subInds[i] = ptrs[i]
			ptrs[i] = &time.Time{}
			continue
		}

		if _, ok := ptr.(*pb.Invocation_State); ok {
			subInds[i] = ptrs[i]
			var tmp int64
			ptrs[i] = &tmp
			continue
		}
	}

	return subInds
}

// replaceValues looks through all the mapped substitutes and replaces them back
// into the original pointer slice, with their correct values.
func replaceValues(ptrs []interface{}, subInds map[int]interface{}) error {
	for i, ptr := range subInds {
		if pOrig, ok := ptr.(**tspb.Timestamp); ok {
			pSub, ok := ptrs[i].(*time.Time)
			if !ok {
				return errors.Reason(
					"expected **timestamp.Timestamp replaced with *time.Time in column %d", i).Err()
			}
			*pOrig = &tspb.Timestamp{
				Seconds: pSub.Unix(),
				Nanos:   int32(pSub.UnixNano() - 1e9*pSub.Unix()),
			}
			ptrs[i] = pOrig
		}

		if pOrig, ok := ptr.(*pb.Invocation_State); ok {
			pSub, ok := ptrs[i].(*int64)
			if !ok {
				return errors.Reason(
					"expected *pb.Invocation_State replaced with *int in column %d", i).Err()
			}
			*pOrig = pb.Invocation_State(*pSub)
			ptrs[i] = pOrig
		}
	}

	return nil
}
