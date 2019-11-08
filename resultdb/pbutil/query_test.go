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

func TestValidateTestResultPredicate(t *testing.T) {
	Convey(`TestValidateTestResultPredicate`, t, func() {
		Convey(`empty`, func() {
			err := ValidateTestResultPredicate(&pb.TestResultPredicate{}, false)
			So(err, ShouldBeNil)
		})

		Convey(`invocation`, func() {
			Convey(`require`, func() {
				err := ValidateTestResultPredicate(&pb.TestResultPredicate{}, true)
				So(err, ShouldErrLike, "invocation: unspecified")
			})

			validate := func(p *pb.InvocationPredicate) error {
				return ValidateTestResultPredicate(&pb.TestResultPredicate{Invocation: p}, false)
			}

			Convey(`name`, func() {
				Convey(`valid`, func() {
					err := validate(&pb.InvocationPredicate{
						RootPredicate: &pb.InvocationPredicate_Name{Name: "invocations/x"},
					})
					So(err, ShouldBeNil)
				})
				Convey(`invalid`, func() {
					err := validate(&pb.InvocationPredicate{
						RootPredicate: &pb.InvocationPredicate_Name{Name: "x"},
					})
					So(err, ShouldErrLike, `invocation: name: does not match`)
				})
			})

			Convey(`tag`, func() {
				Convey(`valid`, func() {
					err := validate(&pb.InvocationPredicate{
						RootPredicate: &pb.InvocationPredicate_Tag{Tag: StringPair("k", "v")},
					})
					So(err, ShouldBeNil)
				})
				Convey(`invalid`, func() {
					err := validate(&pb.InvocationPredicate{
						RootPredicate: &pb.InvocationPredicate_Tag{Tag: StringPair("-", "v")},
					})
					So(err, ShouldErrLike, `invocation: tag: key: does not match`)
				})
			})
		})

		Convey(`test path`, func() {
			validate := func(p *pb.TestPathPredicate) error {
				return ValidateTestResultPredicate(&pb.TestResultPredicate{TestPath: p}, false)
			}

			Convey(`exact`, func() {
				Convey(`valid`, func() {
					err := validate(&pb.TestPathPredicate{
						Predicate: &pb.TestPathPredicate_Exact{Exact: "a"},
					})
					So(err, ShouldBeNil)
				})
				Convey(`invalid`, func() {
					err := validate(&pb.TestPathPredicate{
						Predicate: &pb.TestPathPredicate_Exact{Exact: "\x00"},
					})
					So(err, ShouldErrLike, "test_path: exact: does not match")
				})
			})

			Convey(`prefix`, func() {
				Convey(`valid`, func() {
					err := validate(&pb.TestPathPredicate{
						Predicate: &pb.TestPathPredicate_Prefix{Prefix: "a"},
					})
					So(err, ShouldBeNil)
				})
				Convey(`invalid`, func() {
					err := validate(&pb.TestPathPredicate{
						Predicate: &pb.TestPathPredicate_Prefix{Prefix: "\x00"},
					})
					So(err, ShouldErrLike, "test_path: prefix: does not match")
				})
			})

			Convey(`unspecified`, func() {
				err := validate(&pb.TestPathPredicate{})
				So(err, ShouldErrLike, `test_path: unspecified`)
			})
		})

		Convey(`test variant`, func() {
			validVariant := Variant("a", "b")
			invalidVariant := Variant("", "")

			validate := func(p *pb.VariantPredicate) error {
				return ValidateTestResultPredicate(&pb.TestResultPredicate{Variant: p}, false)
			}

			Convey(`exact`, func() {
				Convey(`valid`, func() {
					err := validate(&pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_Exact{Exact: validVariant},
					})
					So(err, ShouldBeNil)
				})
				Convey(`invalid`, func() {
					err := validate(&pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_Exact{Exact: invalidVariant},
					})
					So(err, ShouldErrLike, `variant: exact: "":"": key: does not match`)
				})
			})

			Convey(`superset_of`, func() {
				Convey(`valid`, func() {
					err := validate(&pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_SupersetOf{SupersetOf: validVariant},
					})
					So(err, ShouldBeNil)
				})
				Convey(`invalid`, func() {
					err := validate(&pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_SupersetOf{SupersetOf: invalidVariant},
					})
					So(err, ShouldErrLike, `variant: superset_of: "":"": key: does not match`)
				})
			})

			Convey(`unspecified`, func() {
				err := validate(&pb.VariantPredicate{})
				So(err, ShouldErrLike, `variant: unspecified`)
			})
		})
	})
}
