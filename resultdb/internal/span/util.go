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
	"sort"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/klauspost/compress/snappy"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/resultdb/internal/metrics"
	internalpb "go.chromium.org/luci/resultdb/internal/proto"
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

// Snappy instructs ToSpanner and FromSpanner functions to encode/decode the
// content with https://godoc.org/github.com/golang/snappy encoding.
type Snappy []byte

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

// Buffer can convert a value from a Spanner type to a Go type.
// Supported types:
//   - string
//   - InvocationID
//   - InvocationIDSet
//   - tspb.Timestamp
//   - pb.InvocationState
//   - pb.TestStatus
//   - pb.Artifact
//   - typepb.Variant
//   - typepb.StringPair
//   - Snappy
type Buffer struct {
	nullStr   spanner.NullString
	nullTime  spanner.NullTime
	i64       int64
	strSlice  []string
	byteSlice []byte
}

// FromSpanner is a shortcut for (&Buffer{}).FromSpanner.
// Appropriate when FromSpanner is called only once, whereas Buffer is reusable
// throughout function.
func FromSpanner(row *spanner.Row, ptrs ...interface{}) error {
	return (&Buffer{}).FromSpanner(row, ptrs...)
}

// FromSpanner reads values from row to ptrs, converting types from Spanner
// to Go along the way.
func (b *Buffer) FromSpanner(row *spanner.Row, ptrs ...interface{}) error {
	if len(ptrs) != row.Size() {
		panic("len(ptrs) != row.Size()")
	}

	for i, goPtr := range ptrs {
		if err := b.fromSpanner(row, i, goPtr); err != nil {
			return err
		}
	}
	return nil
}

func (b *Buffer) fromSpanner(row *spanner.Row, col int, goPtr interface{}) error {
	b.strSlice = b.strSlice[:0]
	b.byteSlice = b.byteSlice[:0]

	var spanPtr interface{}
	switch goPtr.(type) {
	case *string:
		spanPtr = &b.nullStr
	case *InvocationID:
		spanPtr = &b.nullStr
	case *InvocationIDSet:
		spanPtr = &b.strSlice
	case **tspb.Timestamp:
		spanPtr = &b.nullTime
	case *pb.TestStatus:
		spanPtr = &b.i64
	case *pb.Invocation_State:
		spanPtr = &b.i64
	case **typepb.Variant:
		spanPtr = &b.strSlice
	case *[]*typepb.StringPair:
		spanPtr = &b.strSlice
	case *[]*pb.Artifact:
		spanPtr = &b.byteSlice
	case *Snappy:
		spanPtr = &b.byteSlice
	default:
		spanPtr = goPtr
	}

	if err := row.Column(col, spanPtr); err != nil {
		return err
	}

	if spanPtr == goPtr {
		return nil
	}

	var err error
	switch goPtr := goPtr.(type) {
	case *string:
		*goPtr = ""
		if b.nullStr.Valid {
			*goPtr = b.nullStr.StringVal
		}

	case *InvocationID:
		*goPtr = ""
		if b.nullStr.Valid {
			*goPtr = InvocationIDFromRowID(b.nullStr.StringVal)
		}

	case *InvocationIDSet:
		*goPtr = make(InvocationIDSet, len(b.strSlice))
		for _, rowID := range b.strSlice {
			goPtr.Add(InvocationIDFromRowID(rowID))
		}

	case **tspb.Timestamp:
		*goPtr = nil
		if b.nullTime.Valid {
			*goPtr = pbutil.MustTimestampProto(b.nullTime.Time)
		}

	case *pb.Invocation_State:
		*goPtr = pb.Invocation_State(b.i64)

	case *pb.TestStatus:
		*goPtr = pb.TestStatus(b.i64)

	case **typepb.Variant:
		if *goPtr, err = pbutil.VariantFromStrings(b.strSlice); err != nil {
			// If it was written to Spanner, it should have been validated.
			panic(err)
		}

	case *[]*typepb.StringPair:
		*goPtr = make([]*typepb.StringPair, len(b.strSlice))
		for i, p := range b.strSlice {
			if (*goPtr)[i], err = pbutil.StringPairFromString(p); err != nil {
				// If it was written to Spanner, it should have been validated.
				panic(err)
			}
		}

	case *[]*pb.Artifact:
		container := &internalpb.Artifacts{}
		if err := proto.Unmarshal(b.byteSlice, container); err != nil {
			// If it was written to Spanner, it should have been validated.
			panic(err)
		}
		*goPtr = container.ArtifactsV1

	case *Snappy:
		if len(b.byteSlice) == 0 {
			// do not set to nil; otherwise we loose the buffer.
			*goPtr = (*goPtr)[:0]
		} else {
			// *goPtr might be pointing to an existing memory buffer.
			// Try to reuse it for decoding.
			buf := []byte(*goPtr)
			buf = buf[:cap(buf)] // use all capacity
			if *goPtr, err = snappy.Decode(buf, b.byteSlice); err != nil {
				// If it was written to Spanner, it should have been validated.
				panic(errors.Annotate(err, "invalid snappy data: %v", b.byteSlice).Err())
			}
		}

	default:
		panic("impossible")
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

	case InvocationIDSet:
		ret := make([]string, 0, len(v))
		for id := range v {
			ret = append(ret, id.RowID())
		}
		sort.Strings(ret)
		return ret

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
		ret, err := proto.Marshal(&internalpb.Artifacts{ArtifactsV1: v})
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

	case Snappy:
		if len(v) == 0 {
			// Do not store empty bytes.
			return []byte(nil)
		}
		return snappy.Encode(nil, []byte(v))

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

// InsertOrUpdateMap is a shortcut for spanner.InsertOrUpdateMap with ToSpannerMap applied to
// in.
func InsertOrUpdateMap(table string, in map[string]interface{}) *spanner.Mutation {
	return spanner.InsertOrUpdateMap(table, ToSpannerMap(in))
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

// query executes a query.
// Ensures st.Params are Spanner-compatible by modifying st.Params in place.
// Logs the query and the time it took to run it.
func query(ctx context.Context, txn Txn, st spanner.Statement, fn func(row *spanner.Row) error) error {
	// Generate a random query ID in case we have multiple concurrent queries.
	queryID := mathrand.Intn(ctx, 1000)

	defer metrics.Trace(ctx, "query %d", queryID)()

	st.Params = ToSpannerMap(st.Params)
	logging.Infof(ctx, "query %d: %s\n with params %#v", queryID, st.SQL, st.Params)

	return txn.Query(ctx, st).Do(fn)
}
