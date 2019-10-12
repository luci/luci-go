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
	"time"

	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestvalidateCreateInvocationRequest(t *testing.T) {
	t.Parallel()
	now := testclock.TestRecentTimeUTC
	Convey(`TestvalidateCreateInvocationRequest`, t, func() {
		Convey(`empty`, func() {
			err := validateCreateInvocationRequest(&pb.CreateInvocationRequest{}, now)
			So(err, ShouldErrLike, `invocation_id: does not match`)
		})

		Convey(`invalid id`, func() {
			err := validateCreateInvocationRequest(&pb.CreateInvocationRequest{
				InvocationId: "1",
			}, now)
			So(err, ShouldErrLike, `invocation_id: does not match`)
		})

		Convey(`no invocation`, func() {
			err := validateCreateInvocationRequest(&pb.CreateInvocationRequest{
				InvocationId: "a",
			}, now)
			So(err, ShouldBeNil)
		})

		Convey(`invalid tags`, func() {
			err := validateCreateInvocationRequest(&pb.CreateInvocationRequest{
				InvocationId: "abc",
				Invocation: &pb.Invocation{
					Tags: pbutil.StringPairs("1", "a"),
				},
			}, now)
			So(err, ShouldErrLike, `invocation.tags: "1":"a": key: does not match`)
		})

		Convey(`deadline in the past`, func() {
			deadline, err := ptypes.TimestampProto(now.Add(-time.Hour))
			So(err, ShouldBeNil)
			err = validateCreateInvocationRequest(&pb.CreateInvocationRequest{
				InvocationId: "abc",
				Invocation: &pb.Invocation{
					Deadline: deadline,
				},
			}, now)
			So(err, ShouldErrLike, `invocation.deadline must be at least 10 seconds in the future`)
		})

		Convey(`deadline 5s in the future`, func() {
			deadline, err := ptypes.TimestampProto(now.Add(5 * time.Second))
			So(err, ShouldBeNil)
			err = validateCreateInvocationRequest(&pb.CreateInvocationRequest{
				InvocationId: "abc",
				Invocation: &pb.Invocation{
					Deadline: deadline,
				},
			}, now)
			So(err, ShouldErrLike, `invocation.deadline must be at least 10 seconds in the future`)
		})

		Convey(`deadline in the future`, func() {
			deadline, err := ptypes.TimestampProto(now.Add(1e3 * time.Hour))
			So(err, ShouldBeNil)
			err = validateCreateInvocationRequest(&pb.CreateInvocationRequest{
				InvocationId: "abc",
				Invocation: &pb.Invocation{
					Deadline: deadline,
				},
			}, now)
			So(err, ShouldErrLike, `invocation.deadline must be before 48h in the future`)
		})

		Convey(`invalid variant def`, func() {
			err := validateCreateInvocationRequest(&pb.CreateInvocationRequest{
				InvocationId: "abc",
				Invocation: &pb.Invocation{
					BaseTestVariantDef: &pb.VariantDef{
						Def: map[string]string{"1": "a"},
					},
				},
			}, now)
			So(err, ShouldErrLike, `invocation.base_test_variant_def: "1":"a": key: does not match`)
		})

		Convey(`valid`, func() {
			deadline, err := ptypes.TimestampProto(now.Add(time.Hour))
			So(err, ShouldBeNil)

			err = validateCreateInvocationRequest(&pb.CreateInvocationRequest{
				InvocationId: "abc",
				Invocation: &pb.Invocation{
					Deadline: deadline,
					Tags:     pbutil.StringPairs("a", "b", "a", "c", "d", "e"),
					BaseTestVariantDef: &pb.VariantDef{
						Def: map[string]string{
							"a": "b",
							"c": "d",
						},
					},
				},
			}, now)
			So(err, ShouldBeNil)
		})
	})
}
