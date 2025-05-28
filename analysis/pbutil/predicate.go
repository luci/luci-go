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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"

	pb "go.chromium.org/luci/analysis/proto/v1"
)

// ValidateStringPair returns an error if p is invalid.
func ValidateStringPair(p *pb.StringPair) error {
	if err := validate.SpecifiedWithRe(stringPairKeyRe, p.Key); err != nil {
		return errors.Fmt("key: %w", err)
	}
	if len(p.Key) > maxStringPairKeyLength {
		return errors.Fmt("key length must be less or equal to %d", maxStringPairKeyLength)
	}
	if len(p.Value) > maxStringPairValueLength {
		return errors.Fmt("value length must be less or equal to %d", maxStringPairValueLength)
	}
	return nil
}

// ValidateVariant returns an error if vr is invalid.
func ValidateVariant(vr *pb.Variant) error {
	for k, v := range vr.GetDef() {
		p := pb.StringPair{Key: k, Value: v}
		if err := ValidateStringPair(&p); err != nil {
			return errors.Fmt("%q:%q: %w", k, v, err)
		}
	}
	return nil
}

// ValidateVariantPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateVariantPredicate(p *pb.VariantPredicate) error {
	switch pr := p.Predicate.(type) {
	case *pb.VariantPredicate_Equals:
		return errors.WrapIf(ValidateVariant(pr.Equals), "equals")
	case *pb.VariantPredicate_Contains:
		return errors.WrapIf(ValidateVariant(pr.Contains), "contains")
	case *pb.VariantPredicate_HashEquals:
		return errors.WrapIf(validate.SpecifiedWithRe(variantHashRe, pr.HashEquals), "hash_equals")
	case nil:
		return validate.Unspecified()
	default:
		panic("impossible")
	}
}

// ValidateTestVerdictPredicate returns a non-nil error if p is determined to be
// invalid.
func ValidateTestVerdictPredicate(predicate *pb.TestVerdictPredicate) error {
	if predicate == nil {
		return validate.Unspecified()
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
		return errors.Fmt("invalid value %d", value)
	}
	return nil
}

// ValidateTimeRange returns a non-nil error if tr is determined to be invalid.
// To be valid, a TimeRange must have both Earliest and Latest fields specified,
// and the Earliest time must be chronologically before the Latest time.
func ValidateTimeRange(ctx context.Context, tr *pb.TimeRange) error {
	if tr == nil {
		return validate.Unspecified()
	}

	earliest, err := AsTime(tr.Earliest)
	if err != nil {
		return errors.Fmt("earliest: %w", err)
	}

	latest, err := AsTime(tr.Latest)
	if err != nil {
		return errors.Fmt("latest: %w", err)
	}

	if !earliest.Before(latest) {
		return fmt.Errorf("earliest must be before latest")
	}

	return nil
}
