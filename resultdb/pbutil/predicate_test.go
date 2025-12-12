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

func TestValidateWorkUnitPredicate(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateWorkUnitPredicate`, t, func(t *ftt.Test) {
		t.Run(`Valid`, func(t *ftt.Test) {
			p := &pb.WorkUnitPredicate{
				AncestorsOf: "rootIinvocations/a/workUnits/b",
			}
			assert.Loosely(t, ValidateWorkUnitPredicate(p), should.BeNil)
		})

		t.Run(`Nil`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateWorkUnitPredicate(nil), should.ErrLike("unspecified"))
		})

		t.Run(`AncestorsOf unspecified`, func(t *ftt.Test) {
			p := &pb.WorkUnitPredicate{}
			assert.Loosely(t, ValidateWorkUnitPredicate(p), should.ErrLike("ancestors_of: unspecified"))
		})
	})
}

func TestValidateArtifactPredicate(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateArtifactPredicate`, t, func(t *ftt.Test) {
		type testCase struct {
			name                  string
			predicate             *pb.ArtifactPredicate
			isRootInvocationQuery bool
			errLike               string
		}

		testCases := []testCase{
			{
				name: "Root Invocation Query - Valid",
				predicate: &pb.ArtifactPredicate{
					WorkUnits:    []string{"rootInvocations/a/workUnits/b"},
					ArtifactKind: pb.ArtifactPredicate_WORK_UNIT,
				},
				isRootInvocationQuery: true,
			},
			{
				name: "Root Invocation Query - Invalid Work Unit Name",
				predicate: &pb.ArtifactPredicate{
					WorkUnits: []string{"invalid"},
				},
				isRootInvocationQuery: true,
				errLike:               "work_units: \"invalid\": does not match",
			},
			{
				name: "Root Invocation Query - Invalid Artifact Kind",
				predicate: &pb.ArtifactPredicate{
					ArtifactKind: pb.ArtifactPredicate_ArtifactKind(999),
				},
				isRootInvocationQuery: true,
				errLike:               "artifact_kind: invalid value 999",
			},
			{
				name: "Root Invocation Query - FollowEdges set",
				predicate: &pb.ArtifactPredicate{
					FollowEdges: &pb.ArtifactPredicate_EdgeTypeSet{},
				},
				isRootInvocationQuery: true,
				errLike:               "follow_edges: not supported for root invocation queries",
			},
			{
				name: "Legacy Invocation Query - Valid",
				predicate: &pb.ArtifactPredicate{
					FollowEdges: &pb.ArtifactPredicate_EdgeTypeSet{},
				},
				isRootInvocationQuery: false,
			},
			{
				name: "Legacy Invocation Query - WorkUnits set",
				predicate: &pb.ArtifactPredicate{
					WorkUnits: []string{"rootInvocations/a/workUnits/b"},
				},
				isRootInvocationQuery: false,
				errLike:               "work_units: not supported for invocation queries",
			},
			{
				name: "Legacy Invocation Query - ArtifactKind set",
				predicate: &pb.ArtifactPredicate{
					ArtifactKind: pb.ArtifactPredicate_WORK_UNIT,
				},
				isRootInvocationQuery: false,
				errLike:               "artifact_kind: not supported for invocation queries",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *ftt.Test) {
				err := ValidateArtifactPredicate(tc.predicate, tc.isRootInvocationQuery)
				if tc.errLike != "" {
					assert.Loosely(t, err, should.ErrLike(tc.errLike))
				} else {
					assert.Loosely(t, err, should.BeNil)
				}
			})
		}
	})
}
