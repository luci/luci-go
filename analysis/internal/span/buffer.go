// Copyright 2022 The LUCI Authors.
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
	"fmt"
	"reflect"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/errors"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// Value can be converted to a Spanner value.
// Typically if type T implements Value, then *T implements Ptr.
type Value interface {
	// ToSpanner returns a value of a type supported by Spanner client.
	ToSpanner() any
}

// Ptr can be used a destination of reading a Spanner cell.
// Typically if type *T implements Ptr, then T implements Value.
type Ptr interface {
	// SpannerPtr returns to a pointer of a type supported by Spanner client.
	// SpannerPtr must use one of typed buffers in b.
	SpannerPtr(b *Buffer) any
	// FromSpanner replaces Ptr value with the value in the typed buffer returned
	// by SpannerPtr.
	FromSpanner(b *Buffer) error
}

// Buffer can convert a value from a Spanner type to a Go type.
// Supported types:
//   - Value and Ptr
//   - string
//   - timestamppb.Timestamp
//   - atvpb.Status
//   - pb.Variant
//   - pb.StringPair
//   - proto.Message
//   - internal.VerdictStatus
type Buffer struct {
	NullString  spanner.NullString
	NullTime    spanner.NullTime
	Int64       int64
	StringSlice []string
	ByteSlice   []byte
	Int64Slice  []int64
}

// FromSpanner is a shortcut for (&Buffer{}).FromSpanner.
// Appropriate when FromSpanner is called only once, whereas Buffer is reusable
// throughout function.
func FromSpanner(row *spanner.Row, ptrs ...any) error {
	return (&Buffer{}).FromSpanner(row, ptrs...)
}

// FromSpanner reads values from row to ptrs, converting types from Spanner
// to Go along the way.
func (b *Buffer) FromSpanner(row *spanner.Row, ptrs ...any) error {
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

func (b *Buffer) fromSpanner(row *spanner.Row, col int, goPtr any) error {
	b.StringSlice = b.StringSlice[:0]
	b.ByteSlice = b.ByteSlice[:0]
	b.Int64Slice = b.Int64Slice[:0]

	var spanPtr any
	switch goPtr := goPtr.(type) {
	case Ptr:
		spanPtr = goPtr.SpannerPtr(b)
	case *string:
		spanPtr = &b.NullString
	case **timestamppb.Timestamp:
		spanPtr = &b.NullTime
	case *pb.BuildStatus:
		spanPtr = &b.Int64
	case *[]pb.ExonerationReason:
		spanPtr = &b.Int64Slice
	case *pb.TestResultStatus:
		spanPtr = &b.Int64
	case **pb.Variant:
		spanPtr = &b.StringSlice
	case *[]*pb.StringPair:
		spanPtr = &b.StringSlice
	case proto.Message:
		spanPtr = &b.ByteSlice
	default:
		spanPtr = goPtr
	}

	if err := row.Column(col, spanPtr); err != nil {
		return errors.Annotate(err, "failed to read column %q", row.ColumnName(col)).Err()
	}

	if spanPtr == goPtr {
		return nil
	}

	var err error
	switch goPtr := goPtr.(type) {
	case Ptr:
		if err := goPtr.FromSpanner(b); err != nil {
			return err
		}

	case *string:
		*goPtr = ""
		if b.NullString.Valid {
			*goPtr = b.NullString.StringVal
		}

	case **timestamppb.Timestamp:
		*goPtr = nil
		if b.NullTime.Valid {
			*goPtr = pbutil.MustTimestampProto(b.NullTime.Time)
		}

	case *pb.BuildStatus:
		*goPtr = pb.BuildStatus(b.Int64)

	case *[]pb.ExonerationReason:
		*goPtr = nil
		// Be careful to preserve NULLs.
		if b.Int64Slice != nil {
			*goPtr = make([]pb.ExonerationReason, len(b.Int64Slice))
			for i, p := range b.Int64Slice {
				(*goPtr)[i] = pb.ExonerationReason(p)
			}
		}

	case *pb.TestResultStatus:
		*goPtr = pb.TestResultStatus(b.Int64)

	case **pb.Variant:
		if *goPtr, err = pbutil.VariantFromStrings(b.StringSlice); err != nil {
			// If it was written to Spanner, it should have been validated.
			panic(err)
		}

	case *[]*pb.StringPair:
		*goPtr = make([]*pb.StringPair, len(b.StringSlice))
		for i, p := range b.StringSlice {
			if (*goPtr)[i], err = pbutil.StringPairFromString(p); err != nil {
				// If it was written to Spanner, it should have been validated.
				panic(err)
			}
		}

	case proto.Message:
		if reflect.ValueOf(goPtr).IsNil() {
			return errors.Reason("nil pointer encountered").Err()
		}
		if err := proto.Unmarshal(b.ByteSlice, goPtr); err != nil {
			// If it was written to Spanner, it should have been validated.
			panic(err)
		}

	default:
		panic(fmt.Sprintf("impossible %q", goPtr))
	}
	return nil
}

// ToSpanner converts values from Go types to Spanner types. In addition to
// supported types in FromSpanner, also supports []any and
// map[string]any.
func ToSpanner(v any) any {
	switch v := v.(type) {
	case Value:
		return v.ToSpanner()

	case *timestamppb.Timestamp:
		if v == nil {
			return spanner.NullTime{}
		}
		ret := spanner.NullTime{Valid: true}
		ret.Time = v.AsTime()
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

	case pb.BuildStatus:
		return int64(v)

	case []pb.ExonerationReason:
		var spanPtr []int64
		// Be careful to preserve NULLs.
		if v != nil {
			spanPtr = make([]int64, len(v))
			for i, s := range v {
				spanPtr[i] = int64(s)
			}
		}
		return spanPtr

	case pb.TestResultStatus:
		return int64(v)

	case *pb.Variant:
		return pbutil.VariantToStrings(v)

	case []*pb.StringPair:
		return pbutil.StringPairsToStrings(v...)

	case proto.Message:
		if reflect.ValueOf(v).IsNil() {
			// Do not store empty messages.
			return []byte(nil)
		}

		ret, err := proto.Marshal(v)
		if err != nil {
			panic(err)
		}
		return ret

	case []any:
		ret := make([]any, len(v))
		for i, el := range v {
			ret[i] = ToSpanner(el)
		}
		return ret

	case map[string]any:
		ret := make(map[string]any, len(v))
		for key, el := range v {
			ret[key] = ToSpanner(el)
		}
		return ret

	default:
		return v
	}
}

// ToSpannerMap converts a map of Go values to a map of Spanner values.
// See also ToSpanner.
func ToSpannerMap(values map[string]any) map[string]any {
	return ToSpanner(values).(map[string]any)
}

// UpdateMap is a shortcut for spanner.UpdateMap with ToSpannerMap applied to
// in.
func UpdateMap(table string, in map[string]any) *spanner.Mutation {
	return spanner.UpdateMap(table, ToSpannerMap(in))
}

// InsertMap is a shortcut for spanner.InsertMap with ToSpannerMap applied to
// in.
func InsertMap(table string, in map[string]any) *spanner.Mutation {
	return spanner.InsertMap(table, ToSpannerMap(in))
}

// InsertOrUpdateMap is a shortcut for spanner.InsertOrUpdateMap with ToSpannerMap applied to
// in.
func InsertOrUpdateMap(table string, in map[string]any) *spanner.Mutation {
	return spanner.InsertOrUpdateMap(table, ToSpannerMap(in))
}
