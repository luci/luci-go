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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// testObjectPredicate is implemented by both *pb.TestResultPredicate
// and *pb.TestExonerationPredicate.
type testObjectPredicate interface {
	GetTestIdRegexp() string
	GetVariant() *pb.VariantPredicate
}

// validateTestObjectPredicate returns a non-nil error if p is determined to be
// invalid.
func validateTestObjectPredicate(p testObjectPredicate) error {
	if err := validate.RegexpFragment(p.GetTestIdRegexp()); err != nil {
		return errors.Annotate(err, "test_id_regexp").Err()
	}

	if p.GetVariant() != nil {
		if err := ValidateVariantPredicate(p.GetVariant()); err != nil {
			return errors.Annotate(err, "variant").Err()
		}
	}
	return nil
}

// ValidateTestResultPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateTestResultPredicate(p *pb.TestResultPredicate) error {
	if err := ValidateEnum(int32(p.GetExpectancy()), pb.TestResultPredicate_Expectancy_name); err != nil {
		return errors.Annotate(err, "expectancy").Err()
	}

	if p.GetExcludeExonerated() && p.GetExpectancy() == pb.TestResultPredicate_ALL {
		return errors.Reason("exclude_exonerated and expectancy=ALL are mutually exclusive").Err()
	}

	return validateTestObjectPredicate(p)
}

// ValidateTestExonerationPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateTestExonerationPredicate(p *pb.TestExonerationPredicate) error {
	return validateTestObjectPredicate(p)
}

// ValidateVariantPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateVariantPredicate(p *pb.VariantPredicate) error {
	switch pr := p.Predicate.(type) {
	case *pb.VariantPredicate_Equals:
		if pr.Equals == nil {
			// Legacy clients may use a nil variant to mean the empty variant.
			return nil
		}
		return errors.Annotate(ValidateVariant(pr.Equals), "equals").Err()
	case *pb.VariantPredicate_Contains:
		if pr.Contains == nil {
			// Legacy clients may use a nil variant to mean the empty variant.
			return nil
		}
		return errors.Annotate(ValidateVariant(pr.Contains), "contains").Err()
	case nil:
		return validate.Unspecified()
	default:
		panic("impossible")
	}
}

// ValidateArtifactPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateArtifactPredicate(p *pb.ArtifactPredicate) error {
	if err := ValidateTestResultPredicate(p.GetTestResultPredicate()); err != nil {
		return errors.Annotate(err, "text_result_predicate").Err()
	}
	if err := validate.RegexpFragment(p.GetContentTypeRegexp()); err != nil {
		return errors.Annotate(err, "content_type_regexp").Err()
	}
	return nil
}

// ValidateTestMetadataPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateTestMetadataPredicate(p *pb.TestMetadataPredicate) error {
	for i, testID := range p.GetTestIds() {
		if err := ValidateTestID(testID); err != nil {
			return errors.Annotate(err, "test_ids[%v]", i).Err()
		}
	}
	return nil
}
