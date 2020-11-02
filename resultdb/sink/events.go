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

package sink

import (
	"context"
	"time"
)

// EventListener listens sink events that are primarily useful for counting the occurrences
// and meansuring the performance of significant sink operations.
type EventListener interface {
	// RunFinished is invoked when Run returns.
	RunFinished(ctx context.Context, taken time.Duration, err error)

	// ServerWarmedUp is invoked when Server.wramUp returns.
	ServerWarmedUp(ctx context.Context, taken time.Duration, err error)

	// ServerShutdown is invoked when Server.Shutdown returns.
	ServerShutdown(ctx context.Context, taken time.Duration, err error)

	// ArtifactChannelDrained is invoked when the artifact channel was drained completely.
	ArtifactChannelDrained(ctx context.Context, taken time.Duration)

	// TestResultChannelDrained is invoked when the test-result channel was drained completely.
	TestResultChannelDrained(ctx context.Context, taken time.Duration)

	// TestResultsReceived is invoked each time sink receives an HTTP request with
	// test results. If err != nil, nArts may be smaller than the actual number of
	// artifacts included in the request.
	//
	// Note that the implementation of this function MUST be thread-safe. This function
	// may be invoked by more than one goroutines concurrently.
	TestResultsReceived(
		ctx context.Context,
		taken, takenArtEnqueue, takenTREnqueue time.Duration,
		nArt, nTR int, err error,
	)
}

// DummyEventListener is a noop implementation of EventListener interface.
type DummyEventListener struct{}

func (l *DummyEventListener) RunFinished(context.Context, time.Duration, error)       {}
func (l *DummyEventListener) ServerWarmedUp(context.Context, time.Duration, error)    {}
func (l *DummyEventListener) ServerShutdown(context.Context, time.Duration, error)    {}
func (l *DummyEventListener) ArtifactChannelDrained(context.Context, time.Duration)   {}
func (l *DummyEventListener) TestResultChannelDrained(context.Context, time.Duration) {}
func (l *DummyEventListener) TestResultsReceived(
	context.Context, time.Duration, time.Duration, time.Duration, int, int, error,
) {
}
