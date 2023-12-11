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

// Package pbutil contains methods for manipulating LUCI Analysis protos.
package pbutil

import (
	"context"
	"fmt"
	"regexp"
	"regexp/syntax"
	"strings"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/analysis/proto/v1"
)

var (
	// Unspecified is the error to be used when something is unspecified when it's
	// supposed to.
	Unspecified = errors.Reason("unspecified").Err()

	// DoesNotMatch is the error to be used when a string does not match a regex.
	DoesNotMatch = errors.Reason("does not match").Err()
)

// validateRegexp returns a non-nil error if re is an invalid regular
// expression.
func validateRegexp(re string) error {
	// Note: regexp.Compile uses syntax.Perl.
	if _, err := syntax.Parse(re, syntax.Perl); err != nil {
		return err
	}

	// Do not allow ^ and $ in the regexp, because we need to be able to prepend
	// a pattern to the user-supplied pattern.
	if strings.HasPrefix(re, "^") {
		return errors.Reason("must not start with ^; it is prepended automatically").Err()
	}
	if strings.HasSuffix(re, "$") {
		return errors.Reason("must not end with $; it is appended automatically").Err()
	}

	return nil
}

// ValidateWithRe validates a value matches the given re.
func ValidateWithRe(re *regexp.Regexp, value string) error {
	if value == "" {
		return Unspecified
	}
	if !re.MatchString(value) {
		return DoesNotMatch
	}
	return nil
}

// ValidateStringPair returns an error if p is invalid.
func ValidateStringPair(p *pb.StringPair) error {
	if err := ValidateWithRe(stringPairKeyRe, p.Key); err != nil {
		return errors.Annotate(err, "key").Err()
	}
	if len(p.Key) > maxStringPairKeyLength {
		return errors.Reason("key length must be less or equal to %d", maxStringPairKeyLength).Err()
	}
	if len(p.Value) > maxStringPairValueLength {
		return errors.Reason("value length must be less or equal to %d", maxStringPairValueLength).Err()
	}
	return nil
}

// ValidateVariant returns an error if vr is invalid.
func ValidateVariant(vr *pb.Variant) error {
	for k, v := range vr.GetDef() {
		p := pb.StringPair{Key: k, Value: v}
		if err := ValidateStringPair(&p); err != nil {
			return errors.Annotate(err, "%q:%q", k, v).Err()
		}
	}
	return nil
}

// ValidateVariantPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateVariantPredicate(p *pb.VariantPredicate) error {
	switch pr := p.Predicate.(type) {
	case *pb.VariantPredicate_Equals:
		return errors.Annotate(ValidateVariant(pr.Equals), "equals").Err()
	case *pb.VariantPredicate_Contains:
		return errors.Annotate(ValidateVariant(pr.Contains), "contains").Err()
	case *pb.VariantPredicate_HashEquals:
		return errors.Annotate(ValidateWithRe(variantHashRe, pr.HashEquals), "hash_equals").Err()
	case nil:
		return Unspecified
	default:
		panic("impossible")
	}
}

// ValidateTestVerdictPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateTestVerdictPredicate(predicate *pb.TestVerdictPredicate) error {
	if predicate == nil {
		return Unspecified
	}

	if predicate.GetVariantPredicate() != nil {
		if err := ValidateVariantPredicate(predicate.GetVariantPredicate()); err != nil {
			return err
		}
	}
	return ValidateEnum(int32(predicate.GetSubmittedFilter()), pb.SubmittedFilter_name)
}

// ValidateEnum returns a non-nil error if the value is not among valid values.
func ValidateEnum(value int32, validValues map[int32]string) error {
	if _, ok := validValues[value]; !ok {
		return errors.Reason("invalid value %d", value).Err()
	}
	return nil
}

// ValidateTimeRange returns a non-nil error if tr is determined to be invalid.
// To be valid, a TimeRange must have both Earliest and Latest fields specified,
// and the Earliest time must be chronologically before the Latest time.
func ValidateTimeRange(ctx context.Context, tr *pb.TimeRange) error {
	if tr == nil {
		return Unspecified
	}

	earliest, err := AsTime(tr.Earliest)
	if err != nil {
		return errors.Annotate(err, "earliest").Err()
	}

	latest, err := AsTime(tr.Latest)
	if err != nil {
		return errors.Annotate(err, "latest").Err()
	}

	if !earliest.Before(latest) {
		return fmt.Errorf("earliest must be before latest")
	}

	return nil
}
