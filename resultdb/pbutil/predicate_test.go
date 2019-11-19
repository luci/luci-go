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
		Convey(`Empty`, func() {
			err := ValidateTestResultPredicate(&pb.TestResultPredicate{})
			So(err, ShouldErrLike, "invocation: unspecified")
		})

		Convey(`Invocation`, func() {
			validate := func(p *pb.InvocationPredicate) error {
				return ValidateTestResultPredicate(&pb.TestResultPredicate{Invocation: p})
			}

			Convey(`Name`, func() {
				Convey(`Valid`, func() {
					err := validate(&pb.InvocationPredicate{
						Names: []string{"invocations/x", "invocations/y"},
					})
					So(err, ShouldBeNil)
				})
				Convey(`Invalid`, func() {
					err := validate(&pb.InvocationPredicate{
						Names: []string{"invocations/x", "y"},
					})
					So(err, ShouldErrLike, `invocation: name "y": does not match`)
				})
			})

			Convey(`Tag`, func() {
				Convey(`Valid`, func() {
					err := validate(&pb.InvocationPredicate{
						Tags: StringPairs("k", "v"),
					})
					So(err, ShouldBeNil)
				})
				Convey(`Invalid`, func() {
					err := validate(&pb.InvocationPredicate{
						Tags: StringPairs("-", "v"),
					})
					So(err, ShouldErrLike, `invocation: tag "-:v": key: does not match`)
				})
			})
		})

		invPred := &pb.InvocationPredicate{Names: []string{"invocations/inv"}}
		Convey(`Test path`, func() {
			validate := func(p *pb.TestPathPredicate) error {
				return ValidateTestResultPredicate(&pb.TestResultPredicate{
					Invocation: invPred,
					TestPath:   p,
				})
			}

			Convey(`Exact`, func() {
				Convey(`Valid`, func() {
					err := validate(&pb.TestPathPredicate{
						Predicate: &pb.TestPathPredicate_Exact{Exact: "a"},
					})
					So(err, ShouldBeNil)
				})
				Convey(`Invalid`, func() {
					err := validate(&pb.TestPathPredicate{
						Predicate: &pb.TestPathPredicate_Exact{Exact: "\x00"},
					})
					So(err, ShouldErrLike, "test_path: exact: does not match")
				})
			})

			Convey(`Prefix`, func() {
				Convey(`Valid`, func() {
					err := validate(&pb.TestPathPredicate{
						Predicate: &pb.TestPathPredicate_Prefix{Prefix: "a"},
					})
					So(err, ShouldBeNil)
				})
				Convey(`Invalid`, func() {
					err := validate(&pb.TestPathPredicate{
						Predicate: &pb.TestPathPredicate_Prefix{Prefix: "\x00"},
					})
					So(err, ShouldErrLike, "test_path: prefix: does not match")
				})
			})

			Convey(`Unspecified`, func() {
				err := validate(&pb.TestPathPredicate{})
				So(err, ShouldErrLike, `test_path: unspecified`)
			})
		})

		Convey(`Test variant`, func() {
			validVariant := Variant("a", "b")
			invalidVariant := Variant("", "")

			validate := func(p *pb.VariantPredicate) error {
				return ValidateTestResultPredicate(&pb.TestResultPredicate{
					Invocation: invPred,
					Variant:    p,
				})
			}

			Convey(`Exact`, func() {
				Convey(`Valid`, func() {
					err := validate(&pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_Exact{Exact: validVariant},
					})
					So(err, ShouldBeNil)
				})
				Convey(`Invalid`, func() {
					err := validate(&pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_Exact{Exact: invalidVariant},
					})
					So(err, ShouldErrLike, `variant: exact: "":"": key: does not match`)
				})
			})

			Convey(`Contains`, func() {
				Convey(`Valid`, func() {
					err := validate(&pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_Contains{Contains: validVariant},
					})
					So(err, ShouldBeNil)
				})
				Convey(`Invalid`, func() {
					err := validate(&pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_Contains{Contains: invalidVariant},
					})
					So(err, ShouldErrLike, `variant: contains: "":"": key: does not match`)
				})
			})

			Convey(`Unspecified`, func() {
				err := validate(&pb.VariantPredicate{})
				So(err, ShouldErrLike, `variant: unspecified`)
			})
		})
	})
}
