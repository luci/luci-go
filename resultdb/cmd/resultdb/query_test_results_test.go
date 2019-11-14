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

package main

import (
	"testing"

	durpb "github.com/golang/protobuf/ptypes/duration"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateQueryTestResultsRequest(t *testing.T) {
	t.Parallel()
	Convey(`Valid`, t, func() {
		err := validateQueryTestResultsRequest(&pb.QueryTestResultsRequest{
			Predicate: &pb.TestResultPredicate{
				Invocation: &pb.InvocationPredicate{
					RootPredicate: &pb.InvocationPredicate_Name{Name: "invocations/x"},
				},
			},
			PageSize:     50,
			MaxStaleness: &durpb.Duration{Seconds: 60},
		})
		So(err, ShouldBeNil)
	})

	Convey(`invalid predicate`, t, func() {
		err := validateQueryTestResultsRequest(&pb.QueryTestResultsRequest{
			Predicate: &pb.TestResultPredicate{
				Invocation: &pb.InvocationPredicate{
					RootPredicate: &pb.InvocationPredicate_Name{Name: "xxxxxxxxxxxxx"},
				},
			},
		})
		So(err, ShouldErrLike, `predicate: invocation: name: does not match`)
	})
}
