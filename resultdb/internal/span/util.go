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
	"github.com/golang/protobuf/ptypes"
	tspb "github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/resultdb/pbutil"
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

	return NewColumnReader(ptrs...).Read(row)
}

// ColumnReader can read column values and convert them to Go types.
// For example, it can convert Spanner's int64 to pb.InvocationState.
//
// Supported types:
//   - pb.InvocationState
//   - tspb.Timestamp
type ColumnReader struct {
	// pointers to Go values, such as *pb.InvocationState
	goPtrs []interface{}
	// pointers to Spanner values, such as *int64
	spanPtrs []interface{}
}

// NewColumnReader creates a new column reader.
// ptrs is a slice of pointers to read values into.
func NewColumnReader(ptrs ...interface{}) *ColumnReader {
	r := &ColumnReader{
		goPtrs:   ptrs,
		spanPtrs: make([]interface{}, len(ptrs)),
	}

	for i, goPtr := range r.goPtrs {
		switch goPtr.(type) {
		case **tspb.Timestamp:
			r.spanPtrs[i] = &time.Time{}
		case *pb.Invocation_State:
			r.spanPtrs[i] = new(int64)
		case **pb.VariantDef:
			r.spanPtrs[i] = &[]string{}
		case *[]*pb.StringPair:
			r.spanPtrs[i] = &[]string{}
		default:
			r.spanPtrs[i] = goPtr
		}
	}

	return r
}

// Read reads columns from row into ptrs specified in NewColumnReader.
func (r *ColumnReader) Read(row *spanner.Row) error {
	if err := row.Columns(r.spanPtrs...); err != nil {
		return err
	}

	// Declare err to use in short statements.
	var err error
	for i, spanPtr := range r.spanPtrs {
		goPtr := r.goPtrs[i]
		if spanPtr == goPtr {
			continue
		}

		switch goPtr := goPtr.(type) {
		case **tspb.Timestamp:
			if *goPtr, err = ptypes.TimestampProto(*spanPtr.(*time.Time)); err != nil {
				panic(err)
			}

		case *pb.Invocation_State:
			*goPtr = pb.Invocation_State(*spanPtr.(*int64))

		case **pb.VariantDef:
			if *goPtr, err = pbutil.VariantDefFromStrings(*spanPtr.(*[]string)); err != nil {
				// If it was written to Spanner, it should have been validated.
				panic(err)
			}

		case *[]*pb.StringPair:
			pairs := *spanPtr.(*[]string)
			*goPtr = make([]*pb.StringPair, len(pairs))
			for i, p := range pairs {
				if (*goPtr)[i], err = pbutil.StringPairFromString(p); err != nil {
					// If it was written to Spanner, it should have been validated.
					panic(err)
				}
			}

		default:
			panic("impossible")
		}
	}
	return nil
}
