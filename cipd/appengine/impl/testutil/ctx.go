// Copyright 2018 The LUCI Authors.
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

package testutil

import (
	"context"
	"time"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/taskqueue"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

var TestTime = testclock.TestRecentTimeUTC.Round(time.Millisecond)
var TestUser = identity.Identity("user:u@example.com")

func TestingContext() (context.Context, testclock.TestClock, func(string) context.Context) {
	ctx, _ := testclock.UseTime(gaetesting.TestingContext(), TestTime)
	datastore.GetTestable(ctx).AutoIndex(true)
	taskqueue.GetTestable(ctx).CreatePullQueue("bqlog-events")
	as := func(email string) context.Context {
		return auth.WithState(ctx, &authtest.FakeState{
			Identity: identity.Identity("user:" + email),
		})
	}
	return as(TestUser.Email()), clock.Get(ctx).(testclock.TestClock), as
}
