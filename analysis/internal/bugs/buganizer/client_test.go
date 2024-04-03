// Copyright 2024 The LUCI Authors.
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

package buganizer

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuganizerClient(t *testing.T) {
	ctx := context.Background()
	Convey("Buganizer client", t, func() {
		Convey("no value in context means no buganizer client", func() {
			client, err := CreateBuganizerClient(ctx)
			So(err, ShouldBeNil)
			So(client, ShouldBeNil)
		})
		Convey("empty value in context means no buganizer client", func() {
			ctx = context.WithValue(ctx, &BuganizerClientModeKey, "")
			client, err := CreateBuganizerClient(ctx)
			So(err, ShouldBeNil)
			So(client, ShouldBeNil)
		})
		Convey("`disable` mode doesn't create any client", func() {
			ctx = context.WithValue(ctx, &BuganizerClientModeKey, ModeDisable)
			client, err := CreateBuganizerClient(ctx)
			So(err, ShouldBeNil)
			So(client, ShouldBeNil)
		})
	})
}
