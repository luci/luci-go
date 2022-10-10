// Copyright 2022 The LUCI Authors.
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

package listener

import (
	"context"
	"testing"

	"go.chromium.org/luci/cv/internal/changelist"
	listenerpb "go.chromium.org/luci/cv/settings/listener"

	. "github.com/smartystreets/goconvey/convey"
)

type testScheduler struct {
	tasks []*changelist.UpdateCLTask
}

func (sch *testScheduler) Schedule(_ context.Context, t *changelist.UpdateCLTask) error {
	sch.tasks = append(sch.tasks, t)
	return nil
}

func TestGerrit(t *testing.T) {
	t.Parallel()

	Convey("gerritSubscriber", t, func() {
		ctx := context.Background()
		client, closeFn := mockPubSub(ctx)
		defer closeFn()
		settings := &listenerpb.Settings_GerritSubscription{Host: "example.org"}

		Convey("create a reference to subscription", func() {
			sber := newGerritSubscriber(client, &testScheduler{}, &projectFinder{}, settings)
			So(sber.sub.ID(), ShouldEqual, "example.org")
			settings.SubscriptionId = "my-sub"
			sber = newGerritSubscriber(client, &testScheduler{}, &projectFinder{}, settings)
			So(sber.sub.ID(), ShouldEqual, "my-sub")

			Convey("with receive settings", func() {
				settings.ReceiveSettings = &listenerpb.Settings_ReceiveSettings{
					NumGoroutines:          defaultNumGoroutines + 1,
					MaxOutstandingMessages: defaultMaxOutstandingMessages + 1,
				}
				sber = newGerritSubscriber(client, &testScheduler{}, &projectFinder{}, settings)
				So(sber.sameReceiveSettings(settings.ReceiveSettings), ShouldBeTrue)
			})
		})

		Convey("sameGerritSubscriberSettings", func() {
			settings.ReceiveSettings = &listenerpb.Settings_ReceiveSettings{
				NumGoroutines:          1,
				MaxOutstandingMessages: 2,
			}
			sber := newGerritSubscriber(client, &testScheduler{}, &projectFinder{}, settings)
			check := func() bool {
				return sameGerritSubscriberSettings(sber, settings)
			}
			So(check(), ShouldBeTrue)

			Convey("with different receiver settings", func() {
				settings.ReceiveSettings.NumGoroutines++
				So(check(), ShouldBeFalse)
				settings.ReceiveSettings.NumGoroutines--
				settings.ReceiveSettings.MaxOutstandingMessages++
				So(check(), ShouldBeFalse)
				settings.ReceiveSettings.MaxOutstandingMessages--
				So(check(), ShouldBeTrue)
			})
			Convey("with different subscription ID", func() {
				settings.SubscriptionId = "new-sub"
				So(check(), ShouldBeFalse)
			})
			Convey("with different host", func() {
				settings.Host = "example.org"
				settings.SubscriptionId = "example-sub"
				sber := newGerritSubscriber(client, &testScheduler{}, &projectFinder{}, settings)

				// same subscription ID, but different host.
				settings.Host = "example-2.org"
				So(sameGerritSubscriberSettings(sber, settings), ShouldBeFalse)
			})
		})
	})
}
