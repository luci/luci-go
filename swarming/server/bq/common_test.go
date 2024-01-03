// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bq

import (
	"context"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
)

func setup() (context.Context, testclock.TestClock) {
	ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
	ctx = memory.Use(ctx)
	return ctx, tc
}

type mockStreamClient struct {
	streamNameMock string
	appendCalls    int
	appendRowsMock func(ctx context.Context, rows [][]byte) (resultWaiter, error)
	finalizeCalls  int
	finalizeMock   func(ctx context.Context) error
}

func (s *mockStreamClient) appendRows(ctx context.Context, rows [][]byte) (resultWaiter, error) {
	s.appendCalls += 1
	return s.appendRowsMock(ctx, rows)
}

func (s *mockStreamClient) streamName() string {
	return s.streamNameMock
}

func (s *mockStreamClient) finalize(ctx context.Context) error {
	s.finalizeCalls += 1
	return s.finalizeMock(ctx)
}
