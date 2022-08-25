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
	"strings"
	"testing"

	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateAnalyzedTestVariantPredicate(t *testing.T) {
	Convey(`TestValidateAnalyzedTestVariantPredicate`, t, func() {
		Convey(`Empty`, func() {
			err := ValidateAnalyzedTestVariantPredicate(&atvpb.Predicate{})
			So(err, ShouldBeNil)
		})

		Convey(`TestID`, func() {
			validate := func(TestIdRegexp string) error {
				return ValidateAnalyzedTestVariantPredicate(&atvpb.Predicate{
					TestIdRegexp: TestIdRegexp,
				})
			}

			Convey(`empty`, func() {
				So(validate(""), ShouldBeNil)
			})

			Convey(`valid`, func() {
				So(validate("A.+"), ShouldBeNil)
			})

			Convey(`invalid`, func() {
				So(validate(")"), ShouldErrLike, "test_id_regexp: error parsing regex")
			})
			Convey(`^`, func() {
				So(validate("^a"), ShouldErrLike, "test_id_regexp: must not start with ^")
			})
			Convey(`$`, func() {
				So(validate("a$"), ShouldErrLike, "test_id_regexp: must not end with $")
			})
		})

		Convey(`Status`, func() {
			validate := func(s atvpb.Status) error {
				return ValidateAnalyzedTestVariantPredicate(&atvpb.Predicate{
					Status: s,
				})
			}
			Convey(`unspecified`, func() {
				err := validate(atvpb.Status_STATUS_UNSPECIFIED)
				So(err, ShouldBeNil)
			})
			Convey(`invalid`, func() {
				err := validate(atvpb.Status(100))
				So(err, ShouldErrLike, `status: invalid value 100`)
			})
			Convey(`valid`, func() {
				err := validate(atvpb.Status_FLAKY)
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestValidateVariantPredicate(t *testing.T) {
	Convey(`TestValidateVariantPredicate`, t, func() {
		validVariant := Variant("a", "b")
		invalidVariant := Variant("", "")

		validate := func(p *pb.VariantPredicate) error {
			return ValidateAnalyzedTestVariantPredicate(&atvpb.Predicate{
				Variant: p,
			})
		}

		Convey(`Equals`, func() {
			Convey(`Valid`, func() {
				err := validate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Equals{Equals: validVariant},
				})
				So(err, ShouldBeNil)
			})
			Convey(`Invalid`, func() {
				err := validate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Equals{Equals: invalidVariant},
				})
				So(err, ShouldErrLike, `equals: "":"": key: unspecified`)
			})
		})

		Convey(`Contains`, func() {
			Convey(`Valid`, func() {
				err := validate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{Contains: validVariant},
				})
				So(err, ShouldBeNil)
			})
			Convey(`Invalid`, func() {
				err := validate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{Contains: invalidVariant},
				})
				So(err, ShouldErrLike, `contains: "":"": key: unspecified`)
			})
		})

		Convey(`HashEquals`, func() {
			Convey(`Valid`, func() {
				err := validate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_HashEquals{HashEquals: VariantHash(validVariant)},
				})
				So(err, ShouldBeNil)
			})
			Convey(`Empty string`, func() {
				err := validate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_HashEquals{HashEquals: ""},
				})
				So(err, ShouldErrLike, "hash_equals: unspecified")
			})
			Convey(`Upper case`, func() {
				err := validate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_HashEquals{HashEquals: strings.ToUpper(VariantHash(validVariant))},
				})
				So(err, ShouldErrLike, "hash_equals: does not match")
			})
			Convey(`Invalid length`, func() {
				err := validate(&pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_HashEquals{HashEquals: VariantHash(validVariant)[1:]},
				})
				So(err, ShouldErrLike, "hash_equals: does not match")
			})
		})

		Convey(`Unspecified`, func() {
			err := validate(&pb.VariantPredicate{})
			So(err, ShouldErrLike, `unspecified`)
		})
	})
}
