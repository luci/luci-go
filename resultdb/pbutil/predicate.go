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

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// ValidateTestResultPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateTestResultPredicate(p *pb.TestResultPredicate, requireInvocation bool) error {
	if err := ValidateInvocationPredicate(p.GetInvocation(), !requireInvocation); err != nil {
		return errors.Annotate(err, "invocation").Err()
	}

	if p.GetTestPath() != nil {
		if err := ValidateTestPathPredicate(p.TestPath); err != nil {
			return errors.Annotate(err, "test_path").Err()
		}
	}

	if p.GetVariant() != nil {
		if err := ValidateVariantPredicate(p.Variant); err != nil {
			return errors.Annotate(err, "variant").Err()
		}
	}

	if err := ValidateEnum(int32(p.Expectancy), pb.TestResultPredicate_Expectancy_name); err != nil {
		return errors.Annotate(err, "expected_results").Err()
	}

	return nil
}

// ValidateInvocationPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateInvocationPredicate(p *pb.InvocationPredicate, allowEmpty bool) error {
	switch {
	case p.GetTag() != nil && len(p.GetNames()) > 0:
		return errors.Reason("tag and names are mutually exclusive").Err()

	case p.GetTag() != nil:
		return errors.Annotate(ValidateStringPair(p.Tag), "tag").Err()

	case len(p.GetNames()) > 0:
		for _, name := range p.Names {
			if err := ValidateInvocationName(name); err != nil {
				return errors.Annotate(err, "name %q", name).Err()
			}
		}
		return nil

	case !allowEmpty:
		return unspecified()

	default:
		return nil
	}
}

// ValidateTestPathPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateTestPathPredicate(p *pb.TestPathPredicate) error {
	for _, path := range p.Paths {
		if err := ValidateTestPath(path); err != nil {
			return errors.Annotate(err, "path %q", path).Err()
		}
	}

	for _, prefix := range p.PathPrefixes {
		if err := ValidateTestPath(prefix); err != nil {
			return errors.Annotate(err, "prefix %q", prefix).Err()
		}
	}

	return nil
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
