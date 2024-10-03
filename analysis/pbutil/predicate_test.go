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

package pbutil

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestValidateVariantPredicate(t *testing.T) {
	ftt.Run(`TestValidateVariantPredicate`, t, func(t *ftt.Test) {
		validVariant := Variant("a", "b")
		invalidVariant := Variant("", "")

		t.Run(`Equals`, func(t *ftt.Test) {
			t.Run(`Valid`, func(t *ftt.Test) {
				err := ValidateVariantPredicate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Equals{Equals: validVariant},
				})
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run(`Invalid`, func(t *ftt.Test) {
				err := ValidateVariantPredicate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Equals{Equals: invalidVariant},
				})
				assert.Loosely(t, err, should.ErrLike(`equals: "":"": key: unspecified`))
			})
		})

		t.Run(`Contains`, func(t *ftt.Test) {
			t.Run(`Valid`, func(t *ftt.Test) {
				err := ValidateVariantPredicate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{Contains: validVariant},
				})
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run(`Invalid`, func(t *ftt.Test) {
				err := ValidateVariantPredicate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{Contains: invalidVariant},
				})
				assert.Loosely(t, err, should.ErrLike(`contains: "":"": key: unspecified`))
			})
		})

		t.Run(`HashEquals`, func(t *ftt.Test) {
			t.Run(`Valid`, func(t *ftt.Test) {
				err := ValidateVariantPredicate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_HashEquals{HashEquals: VariantHash(validVariant)},
				})
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run(`Empty string`, func(t *ftt.Test) {
				err := ValidateVariantPredicate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_HashEquals{HashEquals: ""},
				})
				assert.Loosely(t, err, should.ErrLike("hash_equals: unspecified"))
			})
			t.Run(`Upper case`, func(t *ftt.Test) {
				err := ValidateVariantPredicate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_HashEquals{HashEquals: strings.ToUpper(VariantHash(validVariant))},
				})
				assert.Loosely(t, err, should.ErrLike("hash_equals: does not match"))
			})
			t.Run(`Invalid length`, func(t *ftt.Test) {
				err := ValidateVariantPredicate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_HashEquals{HashEquals: VariantHash(validVariant)[1:]},
				})
				assert.Loosely(t, err, should.ErrLike("hash_equals: does not match"))
			})
		})

		t.Run(`Unspecified`, func(t *ftt.Test) {
			err := ValidateVariantPredicate(&pb.VariantPredicate{})
			assert.Loosely(t, err, should.ErrLike(`unspecified`))
		})
	})
}

func TestValidateTimeRange(t *testing.T) {
	ftt.Run(`ValidateTimeRange`, t, func(t *ftt.Test) {
		ctx := context.Background()
		now := time.Date(2044, time.April, 4, 4, 4, 4, 4, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)

		t.Run(`Unspecified`, func(t *ftt.Test) {
			err := ValidateTimeRange(ctx, nil)
			assert.Loosely(t, err, should.ErrLike(`unspecified`))
		})
		t.Run(`Earliest unspecified`, func(t *ftt.Test) {
			tr := &pb.TimeRange{
				Latest: timestamppb.New(now.Add(-1 * time.Hour)),
			}
			err := ValidateTimeRange(ctx, tr)
			assert.Loosely(t, err, should.ErrLike(`earliest: unspecified`))
		})
		t.Run(`Latest unspecified`, func(t *ftt.Test) {
			tr := &pb.TimeRange{
				Earliest: timestamppb.New(now.Add(-1 * time.Hour)),
			}
			err := ValidateTimeRange(ctx, tr)
			assert.Loosely(t, err, should.ErrLike(`latest: unspecified`))
		})
		t.Run(`Earliest before Latest`, func(t *ftt.Test) {
			tr := &pb.TimeRange{
				Earliest: timestamppb.New(now.Add(-1 * time.Hour)),
				Latest:   timestamppb.New(now.Add(-2 * time.Hour)),
			}
			err := ValidateTimeRange(ctx, tr)
			assert.Loosely(t, err, should.ErrLike(`earliest must be before latest`))
		})
		t.Run(`Valid`, func(t *ftt.Test) {
			tr := &pb.TimeRange{
				Earliest: timestamppb.New(now.Add(-1 * time.Hour)),
				Latest:   timestamppb.New(now.Add(-1 * time.Microsecond)),
			}
			err := ValidateTimeRange(ctx, tr)
			assert.Loosely(t, err, should.BeNil)
		})
	})
}
