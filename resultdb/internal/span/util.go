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
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	tspb "github.com/golang/protobuf/ptypes/timestamp"

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

	// Generate new pointers slice with substitutions for Spanner-unsupported types.
	subPtrs := replacePointers(ptrs)

	if err := row.Columns(subPtrs...); err != nil {
		return err
	}

	// Assign all the values back into the original pointer map.
	replaceValues(ptrs, subPtrs)
	return nil
}

// replacePointers returns a new slice replacing the pointers to
// Spanner-unsupported types with pointers to ones Spanner does support.
//
// In the case of supported types, ptrs[i] and subPtrs[i] are equivalent.
// In the case of unsupported types, ptrs[i] is typically a pointer to a
// variable, which itself is a pointer, whereas subPtrs[i] is just the pointer
// to the value.
func replacePointers(ptrs []interface{}) []interface{} {
	subPtrs := make([]interface{}, len(ptrs))

	for i, ptr := range ptrs {
		switch ptr.(type) {
		case **tspb.Timestamp:
			subPtrs[i] = &time.Time{}
		case *pb.Invocation_State:
			var tmp int64
			subPtrs[i] = &tmp
		default:
			subPtrs[i] = ptr
		}
	}

	return subPtrs
}

// replaceValues looks through all the mapped substitutes and replaces them back
// into the original pointer slice, with their correct values.
func replaceValues(ptrs []interface{}, subPtrs []interface{}) {
	if len(ptrs) != len(subPtrs) {
		panic(fmt.Sprintf(
			"original ptr slice length (%d) and replacement ptr slice length (%d) should match",
			len(ptrs), len(subPtrs)))
	}

	for i, ptr := range ptrs {
		if ptr == subPtrs[i] {
			continue
		}

		switch pOrig := ptr.(type) {
		case **tspb.Timestamp:
			pSub, ok := subPtrs[i].(*time.Time)
			if !ok {
				panic(fmt.Sprintf(
					"expected **timestamp.Timestamp replaced with *time.Time in column %d", i))
			}
			*pOrig = &tspb.Timestamp{
				Seconds: pSub.Unix(),
				Nanos:   int32(pSub.UnixNano() - 1e9*pSub.Unix()),
			}

		case *pb.Invocation_State:
			pSub, ok := subPtrs[i].(*int64)
			if !ok {
				panic(fmt.Sprintf(
					"expected *pb.Invocation_State replaced with *int in column %d", i))
			}
			*pOrig = pb.Invocation_State(*pSub)

		default:
			panic(fmt.Sprintf(
				"original type should have been unmodified or handled specifically in column %d", i))
		}
	}
}
