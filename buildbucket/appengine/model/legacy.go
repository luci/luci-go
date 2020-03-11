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

package model

import (
	"time"
)

// LegacyStatus is the status of a legacy build request.
type LegacyStatus int

const (
	_ LegacyStatus = iota
	// Scheduled builds may be leased and started.
	Scheduled
	// Started builds are leased and marked as started.
	Started
	// Completed builds are finished and have an associated LegacyResult.
	Completed
)

// LegacyResult is the result of a completed legacy build.
type LegacyResult int

const (
	_ LegacyResult = iota
	// Success means the build completed successfully.
	Success
	// Failure means the build failed and has an associated LegacyFailureReason.
	Failure
	// Canceled means the build was canceled
	// and has an associated LegacyCancelationReason.
	Canceled
)

// LegacyFailureReason is the reason for a legacy build failure.
type LegacyFailureReason int

const (
	_ LegacyFailureReason = iota
	// BuildFailure means the build itself failed.
	BuildFailure
	// BuildbucketFailure means something went wrong within Buildbucket.
	BuildbucketFailure
	// InfraFailure means something went wrong outside the build and Buildbucket.
	InfraFailure
	// InvalidBuildDefinition means the build system rejected the build definition.
	InvalidBuildDefinition
)

// LegacyCancelationReason is the reason for a canceled legacy build.
type LegacyCancelationReason int

const (
	_ LegacyCancelationReason = iota
	// ExplicitlyCanceled means the build was canceled (likely via API call).
	ExplicitlyCanceled
	// TimeoutCanceled means Buildbucket timed the build out.
	TimeoutCanceled
)

// LeaseProperties are properties associated with the legacy leasing API.
type LeaseProperties struct {
	IsLeased bool `gae:"is_leased"`
	// TODO(crbug/1042991): Create datastore.PropertyConverter in server/auth.
	Leasee              []byte    `gae:"leasee"`
	LeaseExpirationDate time.Time `gae:"lease_expiration_date"`
	// LeaseKey is a random value used to verify the leaseholder's identity.
	LeaseKey    int  `gae:"lease_key"`
	NeverLeased bool `gae:"never_leased"`
}

// LegacyProperties are properties of legacy builds.
//
// Parameters and ResultDetails are byte slices interpretable as JSON.
// TODO(crbug/1042991): Create datastore.PropertyConverter for JSON properties.
type LegacyProperties struct {
	LeaseProperties

	CancelationReason LegacyCancelationReason `gae:"cancelation_reason"`
	FailureReason     LegacyFailureReason     `gae:"failure_reason"`
	Parameters        []byte                  `gae:"parameters"`
	Result            LegacyResult            `gae:"result"`
	ResultDetails     []byte                  `gae:"result_details"`
	// ID of the Build this is a retry of.
	RetryOf int          `gae:"retry_of"`
	Status  LegacyStatus `gae:"status"`
	URL     string       `gae:"url,noindex"`
}
