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

	"go.opentelemetry.io/otel/trace"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

var TestTime = testclock.TestRecentTimeUTC.Round(time.Millisecond)
var TestUser = identity.Identity("user:u@example.com")
var TestRequestID = trace.TraceID{1, 2, 3, 4, 5}

func TestingContext(mocks ...authtest.MockedDatum) (context.Context, testclock.TestClock, func(string) context.Context) {
	ctx := memory.Use(context.Background())
	ctx = txndefer.FilterRDS(ctx)
	ctx, _ = testclock.UseTime(ctx, TestTime)
	ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: TestRequestID,
	}))

	datastore.GetTestable(ctx).AutoIndex(true)

	as := func(email string) context.Context {
		return auth.WithState(ctx, &authtest.FakeState{
			Identity: identity.Identity("user:" + email),
			FakeDB:   authtest.NewFakeDB(mocks...),
		})
	}
	return as(TestUser.Email()), clock.Get(ctx).(testclock.TestClock), as
}
