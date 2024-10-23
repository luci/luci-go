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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateTestObjectPredicate(t *testing.T) {
	ftt.Run(`TestValidateTestObjectPredicate`, t, func(t *ftt.Test) {
		t.Run(`Empty`, func(t *ftt.Test) {
			err := validateTestObjectPredicate(&pb.TestResultPredicate{})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`TestID`, func(t *ftt.Test) {
			validate := func(TestIdRegexp string) error {
				return validateTestObjectPredicate(&pb.TestResultPredicate{
					TestIdRegexp: TestIdRegexp,
				})
			}

			t.Run(`empty`, func(t *ftt.Test) {
				assert.Loosely(t, validate(""), should.BeNil)
			})

			t.Run(`valid`, func(t *ftt.Test) {
				assert.Loosely(t, validate("A.+"), should.BeNil)
			})

			t.Run(`invalid`, func(t *ftt.Test) {
				assert.Loosely(t, validate(")"), should.ErrLike("test_id_regexp: error parsing regex"))
			})
			t.Run(`^`, func(t *ftt.Test) {
				assert.Loosely(t, validate("^a"), should.ErrLike("test_id_regexp: must not start with ^"))
			})
			t.Run(`$`, func(t *ftt.Test) {
				assert.Loosely(t, validate("a$"), should.ErrLike("test_id_regexp: must not end with $"))
			})
		})

		t.Run(`Test variant`, func(t *ftt.Test) {
			validVariant := Variant("a", "b")
			invalidVariant := Variant("", "")

			validate := func(p *pb.VariantPredicate) error {
				return validateTestObjectPredicate(&pb.TestResultPredicate{
					Variant: p,
				})
			}

			t.Run(`Equals`, func(t *ftt.Test) {
				t.Run(`Valid`, func(t *ftt.Test) {
					err := validate(&pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_Equals{Equals: validVariant},
					})
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					err := validate(&pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_Equals{Equals: invalidVariant},
					})
					assert.Loosely(t, err, should.ErrLike(`variant: equals: "":"": key: unspecified`))
				})
			})

			t.Run(`Contains`, func(t *ftt.Test) {
				t.Run(`Valid`, func(t *ftt.Test) {
					err := validate(&pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_Contains{Contains: validVariant},
					})
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					err := validate(&pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_Contains{Contains: invalidVariant},
					})
					assert.Loosely(t, err, should.ErrLike(`variant: contains: "":"": key: unspecified`))
				})
			})

			t.Run(`Unspecified`, func(t *ftt.Test) {
				err := validate(&pb.VariantPredicate{})
				assert.Loosely(t, err, should.ErrLike(`variant: unspecified`))
			})
		})
	})
}

func TestValidateTestResultPredicate(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateTestResultPredicate`, t, func(t *ftt.Test) {
		t.Run(`Expectancy and ExcludeExonerated`, func(t *ftt.Test) {
			err := ValidateTestResultPredicate(&pb.TestResultPredicate{ExcludeExonerated: true})
			assert.Loosely(t, err, should.ErrLike("mutually exclusive"))
		})
	})
}
