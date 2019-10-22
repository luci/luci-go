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

	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateDeriveInvocationRequest(t *testing.T) {
	Convey(`TestValidateDeriveInvocationRequest`, t, func() {
		Convey(`Valid`, func() {
			req := &pb.DeriveInvocationRequest{
				SwarmingTask: &pb.DeriveInvocationRequest_SwarmingTask{
					Hostname: "swarming-host.appspot.com",
					Id:       "beeff00d",
				},
			}
			So(validateDeriveInvocationRequest(req), ShouldBeNil)

			Convey(`with base_test_variant populated`, func() {
				req.BaseTestVariant = &pb.VariantDef{Def: map[string]string{
					"k1":               "v1",
					"key/k2":           "v2",
					"key/with/part/k3": "v3",
				}}
				So(validateDeriveInvocationRequest(req), ShouldBeNil)
			})
		})

		Convey(`Invalid swarming_task`, func() {
			Convey(`missing`, func() {
				req := &pb.DeriveInvocationRequest{}
				So(validateDeriveInvocationRequest(req), ShouldErrLike, "swarming_task missing")
			})

			Convey(`missing hostname`, func() {
				req := &pb.DeriveInvocationRequest{
					SwarmingTask: &pb.DeriveInvocationRequest_SwarmingTask{
						Id: "beeff00d",
					},
				}
				So(validateDeriveInvocationRequest(req), ShouldErrLike, "swarming_task.hostname missing")
			})

			Convey(`bad hostname`, func() {
				req := &pb.DeriveInvocationRequest{
					SwarmingTask: &pb.DeriveInvocationRequest_SwarmingTask{
						Hostname: "https://swarming-host.appspot.com",
						Id:       "beeff00d",
					},
				}
				So(validateDeriveInvocationRequest(req), ShouldErrLike,
					"swarming_task.hostname should not have prefix")
			})

			Convey(`missing id`, func() {
				req := &pb.DeriveInvocationRequest{
					SwarmingTask: &pb.DeriveInvocationRequest_SwarmingTask{
						Hostname: "swarming-host.appspot.com",
					},
				}
				So(validateDeriveInvocationRequest(req), ShouldErrLike, "swarming_task.id missing")
			})
		})

		Convey(`Invalid base_test_variant`, func() {
			req := &pb.DeriveInvocationRequest{
				SwarmingTask: &pb.DeriveInvocationRequest_SwarmingTask{
					Hostname: "swarming-host.appspot.com",
					Id:       "beeff00d",
				},
				BaseTestVariant: &pb.VariantDef{Def: map[string]string{"1": "b"}},
			}
			So(validateDeriveInvocationRequest(req), ShouldErrLike, "key: does not match")
		})
	})
}
