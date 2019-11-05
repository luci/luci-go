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

	durationpb "github.com/golang/protobuf/ptypes/duration"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateTestResultQuery(t *testing.T) {
	Convey(`TestValidateTestResultQuery`, t, func() {
		Convey(`empty`, func() {
			err := ValidateTestResultQuery(&pb.TestResultQuery{})
			So(err, ShouldErrLike, "predicate: invocation: unspecified")
		})

		Convey(`invocation`, func() {
			Convey(`name`, func() {
				Convey(`valid`, func() {
					err := ValidateTestResultQuery(&pb.TestResultQuery{
						Predicate: &pb.TestResultPredicate{
							Invocation: &pb.InvocationPredicate{
								Predicate: &pb.InvocationPredicate_Name{Name: "invocations/x"},
							},
						},
					})
					So(err, ShouldBeNil)
				})
				Convey(`invalid`, func() {
					err := ValidateTestResultQuery(&pb.TestResultQuery{
						Predicate: &pb.TestResultPredicate{
							Invocation: &pb.InvocationPredicate{
								Predicate: &pb.InvocationPredicate_Name{Name: "x"},
							},
						},
					})
					So(err, ShouldErrLike, `predicate: invocation: name: does not match`)
				})
			})

			Convey(`tag`, func() {
				Convey(`valid`, func() {
					err := ValidateTestResultQuery(&pb.TestResultQuery{
						Predicate: &pb.TestResultPredicate{
							Invocation: &pb.InvocationPredicate{
								Predicate: &pb.InvocationPredicate_Tag{Tag: StringPair("k", "v")},
							},
						},
					})
					So(err, ShouldBeNil)
				})
				Convey(`invalid`, func() {
					err := ValidateTestResultQuery(&pb.TestResultQuery{
						Predicate: &pb.TestResultPredicate{
							Invocation: &pb.InvocationPredicate{
								Predicate: &pb.InvocationPredicate_Tag{Tag: StringPair("-", "v")},
							},
						},
					})
					So(err, ShouldErrLike, `predicate: invocation: tag: key: does not match`)
				})
			})
		})

		invNamePred := &pb.InvocationPredicate{
			Predicate: &pb.InvocationPredicate_Name{Name: "invocations/x"},
		}

		Convey(`test path`, func() {
			Convey(`exact`, func() {
				Convey(`valid`, func() {
					err := ValidateTestResultQuery(&pb.TestResultQuery{
						Predicate: &pb.TestResultPredicate{
							Invocation: invNamePred,
							TestPath: &pb.TestPathPredicate{
								Predicate: &pb.TestPathPredicate_Exact{Exact: "a"},
							},
						},
					})
					So(err, ShouldBeNil)
				})
				Convey(`invalid`, func() {
					err := ValidateTestResultQuery(&pb.TestResultQuery{
						Predicate: &pb.TestResultPredicate{
							Invocation: invNamePred,
							TestPath: &pb.TestPathPredicate{
								Predicate: &pb.TestPathPredicate_Exact{Exact: "\x00"},
							},
						},
					})
					So(err, ShouldErrLike, "predicate: test_path: exact: does not match")
				})
			})

			Convey(`prefix`, func() {
				Convey(`valid`, func() {
					err := ValidateTestResultQuery(&pb.TestResultQuery{
						Predicate: &pb.TestResultPredicate{
							Invocation: invNamePred,
							TestPath: &pb.TestPathPredicate{
								Predicate: &pb.TestPathPredicate_Prefix{Prefix: "a"},
							},
						},
					})
					So(err, ShouldBeNil)
				})
				Convey(`invalid`, func() {
					err := ValidateTestResultQuery(&pb.TestResultQuery{
						Predicate: &pb.TestResultPredicate{
							Invocation: invNamePred,
							TestPath: &pb.TestPathPredicate{
								Predicate: &pb.TestPathPredicate_Prefix{Prefix: "\x00"},
							},
						},
					})
					So(err, ShouldErrLike, "predicate: test_path: prefix: does not match")
				})
			})

			Convey(`unspecified`, func() {
				err := ValidateTestResultQuery(&pb.TestResultQuery{
					Predicate: &pb.TestResultPredicate{
						Invocation: invNamePred,
						TestPath:   &pb.TestPathPredicate{},
					},
				})
				So(err, ShouldErrLike, `predicate: test_path: unspecified`)
			})
		})

		Convey(`test variant`, func() {
			validVariant := &pb.VariantDef{
				Def: map[string]string{"a": "b"},
			}
			invalidVariant := &pb.VariantDef{
				Def: map[string]string{"": ""},
			}

			Convey(`exact`, func() {
				Convey(`valid`, func() {
					err := ValidateTestResultQuery(&pb.TestResultQuery{
						Predicate: &pb.TestResultPredicate{
							Invocation: invNamePred,
							TestVariant: &pb.VariantPredicate{
								Predicate: &pb.VariantPredicate_Exact{Exact: validVariant},
							},
						},
					})
					So(err, ShouldBeNil)
				})
				Convey(`invalid`, func() {
					err := ValidateTestResultQuery(&pb.TestResultQuery{
						Predicate: &pb.TestResultPredicate{
							Invocation: invNamePred,
							TestVariant: &pb.VariantPredicate{
								Predicate: &pb.VariantPredicate_Exact{Exact: invalidVariant},
							},
						},
					})
					So(err, ShouldErrLike, `predicate: test_variant: exact: "":"": key: does not match`)
				})
			})

			Convey(`superset_of`, func() {
				Convey(`valid`, func() {
					err := ValidateTestResultQuery(&pb.TestResultQuery{
						Predicate: &pb.TestResultPredicate{
							Invocation: invNamePred,
							TestVariant: &pb.VariantPredicate{
								Predicate: &pb.VariantPredicate_SupersetOf{SupersetOf: validVariant},
							},
						},
					})
					So(err, ShouldBeNil)
				})
				Convey(`invalid`, func() {
					err := ValidateTestResultQuery(&pb.TestResultQuery{
						Predicate: &pb.TestResultPredicate{
							Invocation: invNamePred,
							TestVariant: &pb.VariantPredicate{
								Predicate: &pb.VariantPredicate_SupersetOf{SupersetOf: invalidVariant},
							},
						},
					})
					So(err, ShouldErrLike, `predicate: test_variant: superset_of: "":"": key: does not match`)
				})
			})

			Convey(`unspecified`, func() {
				err := ValidateTestResultQuery(&pb.TestResultQuery{
					Predicate: &pb.TestResultPredicate{
						Invocation:  invNamePred,
						TestVariant: &pb.VariantPredicate{},
					},
				})
				So(err, ShouldErrLike, `predicate: test_variant: unspecified`)
			})
		})

		Convey(`max_staleness`, func() {
			Convey(`lower boundary`, func() {
				err := ValidateTestResultQuery(&pb.TestResultQuery{
					Predicate:    &pb.TestResultPredicate{Invocation: invNamePred},
					MaxStaleness: &durationpb.Duration{Seconds: -1},
				})
				So(err, ShouldErrLike, `max_staleness: must between 0 and 30m, inclusive`)
			})
			Convey(`upper boundary`, func() {
				err := ValidateTestResultQuery(&pb.TestResultQuery{
					Predicate:    &pb.TestResultPredicate{Invocation: invNamePred},
					MaxStaleness: &durationpb.Duration{Seconds: 10000},
				})
				So(err, ShouldErrLike, `max_staleness: must between 0 and 30m, inclusive`)
			})
		})
	})
}
