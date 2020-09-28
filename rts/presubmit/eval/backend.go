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

package eval

import (
	"context"
	"time"

	"go.chromium.org/luci/auth"
)

// Backend interface encapsulates specifics of a particular LUCI project.
// See ./chromium for a Chromium backend.
type Backend interface {
	// Name returns the backend name, e.g. "chromium".
	// It is used for cache isolation.
	Name() string

	// RejectedPatchSets retrieves patchsets rejected due to test failures.
	RejectedPatchSets(RejectedPatchSetsRequest) ([]*RejectedPatchSet, error)

	// TestDurationsSample retrieves a sample of test durations.
	// See TestDurationsSampleRequest for more details.
	//
	// The size and freshness of the returned sample must be large/fresh enough to
	// draw statistically significant conclusions.
	TestDurationsSample(TestDurationsSampleRequest) (*TestDurationsSampleResponse, error)
}

// RejectedPatchSetsRequest is a request to retrieve all patchsets
// rejected due to test failures within the given time range,
// along with tests that caused the rejection.
type RejectedPatchSetsRequest struct {
	Context       context.Context
	Authenticator *auth.Authenticator
	StartTime     time.Time
	EndTime       time.Time
}

// RejectedPatchSet is a patchset rejected due to test failures.
type RejectedPatchSet struct {
	Patchset  GerritPatchset `json:"patchset"`
	Timestamp time.Time      `json:"timestamp"`

	// FailedTests are the tests that caused the rejection.
	FailedTests []*Test `json:"failedTests"`
}

// TestDurationsSampleRequest is a request to retrieve a recent sample of test
// durations.
type TestDurationsSampleRequest struct {
	Context       context.Context
	Authenticator *auth.Authenticator
}

// TestDurationsSampleResponse is a response of Backend.TestDurationsSample().
type TestDurationsSampleResponse struct {
	// TestDurations is a representative sample of test durations.
	//
	// Ideally the number of unique Gerrit patchsets is smaller in order to
	// minimize number of requests to Gerrit.
	TestDurations []*TestDuration

	// TTL is for how long to cache this response.
	TTL time.Duration
}

// TestDuration describes how long a test took.
type TestDuration struct {
	Patchset GerritPatchset `json:"patchset"`
	Test     Test           `json:"test"`
	Duration time.Duration  `json:"duration"`
}
