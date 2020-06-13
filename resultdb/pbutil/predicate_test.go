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

package pbutil

import (
	"testing"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateTestObjectPredicate(t *testing.T) {
	Convey(`TestValidateTestObjectPredicate`, t, func() {
		Convey(`Empty`, func() {
			err := validateTestObjectPredicate(&pb.TestResultPredicate{})
			So(err, ShouldBeNil)
		})

		Convey(`TestID`, func() {
			validate := func(TestIdRegexp string) error {
				return validateTestObjectPredicate(&pb.TestResultPredicate{
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

		Convey(`Test variant`, func() {
			validVariant := Variant("a", "b")
			invalidVariant := Variant("", "")

			validate := func(p *pb.VariantPredicate) error {
				return validateTestObjectPredicate(&pb.TestResultPredicate{
					Variant: p,
				})
			}

			Convey(`Valid`, func() {
				err := validate(&pb.VariantPredicate{
					Value: validVariant,
					Op:    pb.VariantPredicate_EQUALS,
				})
				So(err, ShouldBeNil)
			})

			Convey(`Invalid value`, func() {
				err := validate(&pb.VariantPredicate{
					Value: invalidVariant,
				})
				So(err, ShouldErrLike, `variant: value: "":"": key: unspecified`)
			})

			Convey(`Invalid op`, func() {
				err := validate(&pb.VariantPredicate{
					Value: validVariant,
					Op:    999,
				})
				So(err, ShouldErrLike, `variant: op: invalid value 999`)
			})
		})
	})
}
