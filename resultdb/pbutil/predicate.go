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
		return errors.Fmt("test_id_regexp: %w", err)
	}

	if p.GetVariant() != nil {
		if err := ValidateVariantPredicate(p.GetVariant()); err != nil {
			return errors.Fmt("variant: %w", err)
		}
	}
	return nil
}

// ValidateTestResultPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateTestResultPredicate(p *pb.TestResultPredicate) error {
	if err := ValidateEnum(int32(p.GetExpectancy()), pb.TestResultPredicate_Expectancy_name); err != nil {
		return errors.Fmt("expectancy: %w", err)
	}

	if p.GetExcludeExonerated() && p.GetExpectancy() == pb.TestResultPredicate_ALL {
		return errors.New("exclude_exonerated and expectancy=ALL are mutually exclusive")
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
		return errors.WrapIf(ValidateVariant(pr.Equals), "equals")
	case *pb.VariantPredicate_Contains:
		if pr.Contains == nil {
			// Legacy clients may use a nil variant to mean the empty variant.
			return nil
		}
		return errors.WrapIf(ValidateVariant(pr.Contains), "contains")
	case nil:
		return validate.Unspecified()
	default:
		panic("impossible")
	}
}

// ValidateArtifactPredicate returns a non-nil error if p is determined to be
// invalid.
//
// isRootInvocationQuery indicates if the predicate is used for a root
// invocation query.
func ValidateArtifactPredicate(p *pb.ArtifactPredicate, isRootInvocationQuery bool) error {
	if p == nil {
		return nil
	}
	if err := ValidateTestResultPredicate(p.GetTestResultPredicate()); err != nil {
		return errors.Fmt("text_result_predicate: %w", err)
	}
	if err := validate.RegexpFragment(p.GetContentTypeRegexp()); err != nil {
		return errors.Fmt("content_type_regexp: %w", err)
	}

	if isRootInvocationQuery {
		// Validates work unit names.
		for _, name := range p.WorkUnits {
			if err := ValidateWorkUnitName(name); err != nil {
				return errors.Fmt("work_units: %q: %w", name, err)
			}
		}

		// Validate the specified enum value.
		if p.ArtifactKind != pb.ArtifactPredicate_ARTIFACT_KIND_UNSPECIFIED {
			if _, ok := pb.ArtifactPredicate_ArtifactKind_name[int32(p.ArtifactKind)]; !ok {
				return errors.Fmt("artifact_kind: invalid value %d", p.ArtifactKind)
			}
		}

		if p.FollowEdges != nil {
			return errors.New("follow_edges: not supported for root invocation queries")
		}
	} else {
		if len(p.WorkUnits) > 0 {
			return errors.New("work_units: not supported for invocation queries")
		}
		if p.ArtifactKind != pb.ArtifactPredicate_ARTIFACT_KIND_UNSPECIFIED {
			return errors.New("artifact_kind: not supported for invocation queries")
		}
	}
	return nil
}

// ValidateTestMetadataPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateTestMetadataPredicate(p *pb.TestMetadataPredicate) error {
	for i, testID := range p.GetTestIds() {
		if err := ValidateTestID(testID); err != nil {
			return errors.Fmt("test_ids[%v]: %w", i, err)
		}
	}
	for i, testID := range p.GetPreviousTestIds() {
		if err := ValidateTestID(testID); err != nil {
			return errors.Fmt("previous_test_ids[%v]: %w", i, err)
		}
	}
	if len(p.GetTestIds()) > 0 && len(p.GetPreviousTestIds()) > 0 {
		return errors.New("either test_ids or previous_test_ids may be specified; not both")
	}
	return nil
}

// ValidateWorkUnitPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateWorkUnitPredicate(p *pb.WorkUnitPredicate) error {
	if p == nil {
		return errors.New("unspecified")
	}
	if p.AncestorsOf == "" {
		return errors.New("ancestors_of: unspecified")
	}
	return nil
}
