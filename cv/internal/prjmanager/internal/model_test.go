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

package internal

import (
	"testing"

	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestEvent(t *testing.T) {
	t.Parallel()

	Convey("Event send/receive/delete works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "infra"
		payload := &Payload{Event: &Payload_UpdateConfig{UpdateConfig: &UpdateConfig{}}}

		Convey("single", func() {
			So(Send(ctx, lProject, payload), ShouldBeNil)
			events, err := Peek(ctx, lProject, 100)
			So(err, ShouldBeNil)
			So(events, ShouldHaveLength, 1)
			So(events[0].Payload, ShouldResembleProto, payload)

			So(Delete(ctx, events), ShouldBeNil)
			events, err = Peek(ctx, lProject, 100)
			So(err, ShouldBeNil)
			So(events, ShouldHaveLength, 0)
		})

		Convey("many", func() {
			for i := 0; i < 300; i++ {
				So(Send(ctx, lProject, payload), ShouldBeNil)
			}
			events50, err := Peek(ctx, lProject, 50)
			So(err, ShouldBeNil)
			So(events50, ShouldHaveLength, 50)

			So(Delete(ctx, events50), ShouldBeNil)
			events250, err := Peek(ctx, lProject, 300)
			So(err, ShouldBeNil)
			So(events250, ShouldHaveLength, 250)
			So(Delete(ctx, events250), ShouldBeNil)
		})
	})
}
