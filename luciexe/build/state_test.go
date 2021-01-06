// Copyright 2020 The LUCI Authors.
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

package build

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/assertions"
)

func TestState(t *testing.T) {
	Convey(`State`, t, func() {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		nowpb := timestamppb.New(testclock.TestRecentTimeUTC)
		st, ctx := Start(ctx, nil)
		defer func() {
			if st != nil {
				st.End(nil)
			}
		}()

		Convey(`StartStep`, func() {
			step, _ := StartStep(ctx, "some step")
			defer func() { step.End(nil) }()

			So(st.buildPb.Steps, assertions.ShouldResembleProto, []*bbpb.Step{
				{Name: "some step", StartTime: nowpb, Status: bbpb.Status_STARTED},
			})
		})

		Convey(`End`, func() {
			Convey(`cannot End twice`, func() {
				st.End(nil)
				So(func() { st.End(nil) }, assertions.ShouldPanicLike, "cannot mutate ended build")
				st = nil
			})
		})
	})
}
