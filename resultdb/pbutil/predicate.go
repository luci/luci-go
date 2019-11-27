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
	"regexp/syntax"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// ValidateTestResultPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateTestResultPredicate(p *pb.TestResultPredicate) error {
	if err := ValidateEnum(int32(p.Expectancy), pb.TestResultPredicate_Expectancy_name); err != nil {
		return errors.Annotate(err, "expectancy").Err()
	}

	return validateTestObjectPredicate(p)
}

// ValidateTestExonerationPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateTestExonerationPredicate(p *pb.TestExonerationPredicate) error {
	return validateTestObjectPredicate(p)
}

// ValidateInvocationPredicate returns a non-nil error if p is determined to be
// invalid.
//
// Requires that p has tags or names.
func ValidateInvocationPredicate(p *pb.InvocationPredicate) error {
	if len(p.GetNames()) == 0 && len(p.GetTags()) == 0 {
		return unspecified()
	}

	for _, name := range p.GetNames() {
		if err := ValidateInvocationName(name); err != nil {
			return errors.Annotate(err, "name %q", name).Err()
		}
	}

	for _, tag := range p.GetTags() {
		if err := ValidateStringPair(tag); err != nil {
			return errors.Annotate(err, "tag %q", StringPairToString(tag)).Err()
		}
	}

	return nil
}

// validateRegexp returns a non-nil error if re is an invalid regular
// expression.
func validateRegexp(re string) error {
	// Note: regexp.Compile uses syntax.Perl.
	_, err := syntax.Parse(re, syntax.Perl)
	return err
}

// ValidateVariantPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateVariantPredicate(p *pb.VariantPredicate) error {
	switch pr := p.Predicate.(type) {
	case *pb.VariantPredicate_Exact:
		return errors.Annotate(ValidateVariant(pr.Exact), "exact").Err()
	case *pb.VariantPredicate_Contains:
		return errors.Annotate(ValidateVariant(pr.Contains), "contains").Err()
	case nil:
		return unspecified()
	default:
		panic("impossible")
	}
}
