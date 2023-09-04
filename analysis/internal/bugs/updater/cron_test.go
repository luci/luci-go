// Copyright 2023 The LUCI Authors.
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

package updater

import (
	"context"
	"testing"

	"go.chromium.org/luci/analysis/internal/bugs/buganizer"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCron(t *testing.T) {
	ctx := context.Background()
	Convey("Buganizer client", t, func() {
		Convey("no value in context means no buganizer client", func() {
			client, err := createBuganizerClient(ctx)
			So(err, ShouldBeNil)
			So(client, ShouldBeNil)
		})

		Convey("`disable` mode doesn't create any client", func() {
			ctx = context.WithValue(ctx, &buganizer.BuganizerClientModeKey, buganizer.ModeDisable)
			client, err := createBuganizerClient(ctx)
			So(err, ShouldBeNil)
			So(client, ShouldBeNil)
		})
	})
}
