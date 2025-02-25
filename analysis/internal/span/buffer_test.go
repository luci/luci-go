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
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestTypeConversion(t *testing.T) {
	t.Parallel()

	var b Buffer

	test := func(goValue, spValue any) {
		// ToSpanner
		actualSPValue := ToSpanner(goValue)
		assert.Loosely(t, actualSPValue, should.Match(spValue))

		// FromSpanner
		row, err := spanner.NewRow([]string{"a"}, []any{actualSPValue})
		assert.Loosely(t, err, should.BeNil)
		goPtr := reflect.New(reflect.TypeOf(goValue))
		err = b.FromSpanner(row, goPtr.Interface())
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, goPtr.Elem().Interface(), should.Match(goValue))
	}

	ftt.Run(`int64`, t, func(t *ftt.Test) {
		test(int64(42), int64(42))
	})

	ftt.Run(`*timestamppb.Timestamp`, t, func(t *ftt.Test) {
		test(
			&timestamppb.Timestamp{Seconds: 1000, Nanos: 1234},
			spanner.NullTime{Valid: true, Time: time.Unix(1000, 1234).UTC()},
		)
	})

	ftt.Run(`pb.BuildStatus`, t, func(t *ftt.Test) {
		test(pb.BuildStatus_BUILD_STATUS_SUCCESS, int64(1))
	})
	ftt.Run(`[]pb.ExonerationReason`, t, func(t *ftt.Test) {
		test(
			[]pb.ExonerationReason{
				pb.ExonerationReason_OCCURS_ON_MAINLINE,
				pb.ExonerationReason_NOT_CRITICAL,
			},
			[]int64{int64(1), int64(3)},
		)
		test(
			[]pb.ExonerationReason{},
			[]int64{},
		)
		test(
			[]pb.ExonerationReason(nil),
			[]int64(nil),
		)
	})
	ftt.Run(`pb.TestResultStatus`, t, func(t *ftt.Test) {
		test(pb.TestResultStatus_PASS, int64(1))
	})

	ftt.Run(`*pb.Variant`, t, func(t *ftt.Test) {
		t.Run(`Works`, func(t *ftt.Test) {
			test(
				pbutil.Variant("a", "1", "b", "2"),
				[]string{"a:1", "b:2"},
			)
		})
		t.Run(`Empty`, func(t *ftt.Test) {
			test(
				(*pb.Variant)(nil),
				[]string{},
			)
		})
	})

	ftt.Run(`[]*pb.StringPair`, t, func(t *ftt.Test) {
		test(
			pbutil.StringPairs("a", "1", "b", "2"),
			[]string{"a:1", "b:2"},
		)
	})

	ftt.Run(`Compressed`, t, func(t *ftt.Test) {
		t.Run(`Empty`, func(t *ftt.Test) {
			test(Compressed(nil), []byte(nil))
		})
		t.Run(`non-Empty`, func(t *ftt.Test) {
			test(
				Compressed("aaaaaaaaaaaaaaaaaaaa"),
				[]byte{122, 116, 100, 10, 40, 181, 47, 253, 4, 0, 163, 0, 0, 97, 247, 175, 71, 227})
		})
	})

	ftt.Run(`Map`, t, func(t *ftt.Test) {
		var varIntA, varIntB int64
		var varStatus pb.BuildStatus

		row, err := spanner.NewRow([]string{"a", "b", "c"}, []any{int64(42), int64(56), int64(0)})
		assert.Loosely(t, err, should.BeNil)
		err = b.FromSpanner(row, &varIntA, &varIntB, &varStatus)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, varIntA, should.Equal(42))
		assert.Loosely(t, varIntB, should.Equal(56))
		assert.Loosely(t, varStatus, should.Equal(pb.BuildStatus_BUILD_STATUS_UNSPECIFIED))

		// ToSpanner
		spValues := ToSpannerMap(map[string]any{
			"a": varIntA,
			"b": varIntB,
			"c": varStatus,
		})
		assert.Loosely(t, spValues, should.Match(map[string]any{"a": int64(42), "b": int64(56), "c": int64(0)}))
	})

	ftt.Run(`proto.Message`, t, func(t *ftt.Test) {
		msg := &pb.FailureReason{
			PrimaryErrorMessage: "primary error message",
		}
		expected, err := proto.Marshal(msg)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ToSpanner(msg), should.Match(expected))

		row, err := spanner.NewRow([]string{"a"}, []any{expected})
		assert.Loosely(t, err, should.BeNil)

		t.Run(`success`, func(t *ftt.Test) {
			expectedPtr := &pb.FailureReason{}
			err = b.FromSpanner(row, expectedPtr)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, expectedPtr, should.Match(msg))
		})

		t.Run(`Passing nil pointer to fromSpanner`, func(t *ftt.Test) {
			var expectedPtr *pb.FailureReason
			err = b.FromSpanner(row, expectedPtr)
			assert.Loosely(t, err, should.ErrLike("nil pointer encountered"))
		})
	})
}
