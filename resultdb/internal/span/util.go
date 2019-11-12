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
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	typepb "go.chromium.org/luci/resultdb/proto/type"
)

// This file implements utility functions that make spanner API slightly easier
// to use.

// Txn is implemented by all spanner transactions.
type Txn interface {
	// ReadRow reads a single row from the database.
	ReadRow(ctx context.Context, table string, key spanner.Key, columns []string) (*spanner.Row, error)

	// Read reads multiple rows from the database.
	Read(ctx context.Context, table string, key spanner.KeySet, columns []string) *spanner.RowIterator

	// ReadOptions reads multiple rows from the database, and allows customizing
	// options.
	ReadWithOptions(ctx context.Context, table string, keys spanner.KeySet, columns []string, opts *spanner.ReadOptions) (ri *spanner.RowIterator)

	// Query reads multiple rows returned by SQL statement.
	Query(ctx context.Context, statement spanner.Statement) *spanner.RowIterator
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

	return FromSpanner(row, ptrs...)
}

// FromSpanner reads values from row to ptrs, converting types from Spanner
// to Go along the way.
// Supported types:
//   - string
//   - tspb.Timestamp
//   - pb.InvocationState
//   - pb.TestStatus
//   - pb.Artifact
//   - typepb.Variant
//   - typepb.StringPair
func FromSpanner(row *spanner.Row, ptrs ...interface{}) error {
	spanPtrs := make([]interface{}, len(ptrs))

	for i, goPtr := range ptrs {
		switch goPtr.(type) {
		case *string:
			spanPtrs[i] = &spanner.NullString{}
		case *InvocationID:
			spanPtrs[i] = &spanner.NullString{}
		case **tspb.Timestamp:
			spanPtrs[i] = &spanner.NullTime{}
		case *pb.TestStatus:
			spanPtrs[i] = new(int64)
		case *pb.Invocation_State:
			spanPtrs[i] = new(int64)
		case **typepb.Variant:
			spanPtrs[i] = &[]string{}
		case *[]*typepb.StringPair:
			spanPtrs[i] = &[]string{}
		case *[]*pb.Artifact:
			spanPtrs[i] = &[][]byte{}
		default:
			spanPtrs[i] = goPtr
		}
	}

	if err := row.Columns(spanPtrs...); err != nil {
		return err
	}

	// Declare err to use in short statements.
	var err error
	for i, spanPtr := range spanPtrs {
		goPtr := ptrs[i]
		if spanPtr == goPtr {
			continue
		}

		switch goPtr := goPtr.(type) {
		case *string:
			*goPtr = ""
			if maybe := *spanPtr.(*spanner.NullString); maybe.Valid {
				*goPtr = maybe.StringVal
			}

		case *InvocationID:
			*goPtr = ""
			if maybe := *spanPtr.(*spanner.NullString); maybe.Valid {
				*goPtr = InvocationIDFromRowID(maybe.StringVal)
			}

		case **tspb.Timestamp:
			*goPtr = nil
			if maybe := *spanPtr.(*spanner.NullTime); maybe.Valid {
				*goPtr = pbutil.MustTimestampProto(maybe.Time)
			}

		case *pb.Invocation_State:
			*goPtr = pb.Invocation_State(*spanPtr.(*int64))

		case *pb.TestStatus:
			*goPtr = pb.TestStatus(*spanPtr.(*int64))

		case **typepb.Variant:
			if *goPtr, err = pbutil.VariantFromStrings(*spanPtr.(*[]string)); err != nil {
				// If it was written to Spanner, it should have been validated.
				panic(err)
			}

		case *[]*typepb.StringPair:
			pairs := *spanPtr.(*[]string)
			*goPtr = make([]*typepb.StringPair, len(pairs))
			for i, p := range pairs {
				if (*goPtr)[i], err = pbutil.StringPairFromString(p); err != nil {
					// If it was written to Spanner, it should have been validated.
					panic(err)
				}
			}

		case *[]*pb.Artifact:
			byteSlices := *spanPtr.(*[][]byte)
			if *goPtr, err = pbutil.ArtifactsFromByteSlices(byteSlices); err != nil {
				// If it was written to Spanner, it should have been validated.
				panic(err)
			}

		default:
			panic("impossible")
		}
	}
	return nil
}

// ToSpanner converts values from Go types to Spanner types. In addition to
// supported types in FromSpanner, also supports []interface{} and
// map[string]interface{}.
func ToSpanner(v interface{}) interface{} {
	switch v := v.(type) {
	case InvocationID:
		return v.RowID()

	case *tspb.Timestamp:
		if v == nil {
			return spanner.NullTime{}
		}
		ret := spanner.NullTime{Valid: true}
		ret.Time, _ = ptypes.Timestamp(v)
		// ptypes.Timestamp always returns a timestamp, even
		// if the returned err is non-nil, see its documentation.
		// The error is returned only if the timestamp violates its
		// own invariants, which will be caught on the attempt to
		// insert this value into Spanner.
		// This is consistent with the behavior of spanner.Insert() and
		// other mutating functions that ignore invalid time.Time
		// until it is time to apply the mutation.
		// Not returning an error here significantly simplifies usage
		// of this function and functions based on this one.
		return ret

	case pb.Invocation_State:
		return int64(v)

	case pb.TestStatus:
		return int64(v)

	case *typepb.Variant:
		return pbutil.VariantToStrings(v)

	case []*typepb.StringPair:
		return pbutil.StringPairsToStrings(v...)

	case []*pb.Artifact:
		ret, err := pbutil.ArtifactsToByteSlices(v)
		if err != nil {
			panic(err)
		}
		return ret

	case []interface{}:
		ret := make([]interface{}, len(v))
		for i, el := range v {
			ret[i] = ToSpanner(el)
		}
		return ret

	case map[string]interface{}:
		ret := make(map[string]interface{}, len(v))
		for key, el := range v {
			ret[key] = ToSpanner(el)
		}
		return ret

	default:
		return v
	}
}

// ToSpannerSlice converts a slice of Go values to a slice of Spanner values.
// See also ToSpanner.
func ToSpannerSlice(values ...interface{}) []interface{} {
	return ToSpanner(values).([]interface{})
}

// ToSpannerMap converts a map of Go values to a map of Spanner values.
// See also ToSpanner.
func ToSpannerMap(values map[string]interface{}) map[string]interface{} {
	return ToSpanner(values).(map[string]interface{})
}

// UpdateMap is a shortcut for spanner.UpdateMap with ToSpannerMap applied to
// in.
func UpdateMap(table string, in map[string]interface{}) *spanner.Mutation {
	return spanner.UpdateMap(table, ToSpannerMap(in))
}

// InsertMap is a shortcut for spanner.InsertMap with ToSpannerMap applied to
// in.
func InsertMap(table string, in map[string]interface{}) *spanner.Mutation {
	return spanner.InsertMap(table, ToSpannerMap(in))
}

// ReadWriteTransaction calls Client(ctx).ReadWriteTransaction and unwraps
// a "transaction is aborted" errors such that the spanner client properly
// retries the function.
func ReadWriteTransaction(ctx context.Context, f func(context.Context, *spanner.ReadWriteTransaction) error) (commitTimestamp time.Time, err error) {
	return Client(ctx).ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		err := f(ctx, txn)
		if unwrapped := errors.Unwrap(err); spanner.ErrCode(unwrapped) == codes.Aborted {
			err = unwrapped
		}
		return err
	})
}
