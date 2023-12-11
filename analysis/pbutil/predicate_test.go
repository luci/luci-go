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

	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateVariantPredicate(t *testing.T) {
	Convey(`TestValidateVariantPredicate`, t, func() {
		validVariant := Variant("a", "b")
		invalidVariant := Variant("", "")

		Convey(`Equals`, func() {
			Convey(`Valid`, func() {
				err := ValidateVariantPredicate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Equals{Equals: validVariant},
				})
				So(err, ShouldBeNil)
			})
			Convey(`Invalid`, func() {
				err := ValidateVariantPredicate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Equals{Equals: invalidVariant},
				})
				So(err, ShouldErrLike, `equals: "":"": key: unspecified`)
			})
		})

		Convey(`Contains`, func() {
			Convey(`Valid`, func() {
				err := ValidateVariantPredicate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{Contains: validVariant},
				})
				So(err, ShouldBeNil)
			})
			Convey(`Invalid`, func() {
				err := ValidateVariantPredicate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{Contains: invalidVariant},
				})
				So(err, ShouldErrLike, `contains: "":"": key: unspecified`)
			})
		})

		Convey(`HashEquals`, func() {
			Convey(`Valid`, func() {
				err := ValidateVariantPredicate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_HashEquals{HashEquals: VariantHash(validVariant)},
				})
				So(err, ShouldBeNil)
			})
			Convey(`Empty string`, func() {
				err := ValidateVariantPredicate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_HashEquals{HashEquals: ""},
				})
				So(err, ShouldErrLike, "hash_equals: unspecified")
			})
			Convey(`Upper case`, func() {
				err := ValidateVariantPredicate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_HashEquals{HashEquals: strings.ToUpper(VariantHash(validVariant))},
				})
				So(err, ShouldErrLike, "hash_equals: does not match")
			})
			Convey(`Invalid length`, func() {
				err := ValidateVariantPredicate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_HashEquals{HashEquals: VariantHash(validVariant)[1:]},
				})
				So(err, ShouldErrLike, "hash_equals: does not match")
			})
		})

		Convey(`Unspecified`, func() {
			err := ValidateVariantPredicate(&pb.VariantPredicate{})
			So(err, ShouldErrLike, `unspecified`)
		})
	})
}

func TestValidateTimeRange(t *testing.T) {
	Convey(`ValidateTimeRange`, t, func() {
		ctx := context.Background()
		now := time.Date(2044, time.April, 4, 4, 4, 4, 4, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)

		Convey(`Unspecified`, func() {
			err := ValidateTimeRange(ctx, nil)
			So(err, ShouldErrLike, `unspecified`)
		})
		Convey(`Earliest unspecified`, func() {
			tr := &pb.TimeRange{
				Latest: timestamppb.New(now.Add(-1 * time.Hour)),
			}
			err := ValidateTimeRange(ctx, tr)
			So(err, ShouldErrLike, `earliest: unspecified`)
		})
		Convey(`Latest unspecified`, func() {
			tr := &pb.TimeRange{
				Earliest: timestamppb.New(now.Add(-1 * time.Hour)),
			}
			err := ValidateTimeRange(ctx, tr)
			So(err, ShouldErrLike, `latest: unspecified`)
		})
		Convey(`Earliest before Latest`, func() {
			tr := &pb.TimeRange{
				Earliest: timestamppb.New(now.Add(-1 * time.Hour)),
				Latest:   timestamppb.New(now.Add(-2 * time.Hour)),
			}
			err := ValidateTimeRange(ctx, tr)
			So(err, ShouldErrLike, `earliest must be before latest`)
		})
		Convey(`Valid`, func() {
			tr := &pb.TimeRange{
				Earliest: timestamppb.New(now.Add(-1 * time.Hour)),
				Latest:   timestamppb.New(now.Add(-1 * time.Microsecond)),
			}
			err := ValidateTimeRange(ctx, tr)
			So(err, ShouldBeNil)
		})
	})
}
