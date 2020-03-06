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

type LegacyStatus int

const (
	_ LegacyStatus = iota
	Scheduled
	Started
	Completed
)

type LegacyResult int

const (
	_ LegacyResult = iota
	Success
	Failure
	Canceled
)

type LegacyFailureReason int

const (
	_ LegacyFailureReason = iota
	BuildFailure
	BuildbucketFailure
	InfraFailure
	InvalidBuildDefinition
)

type LegacyCancelationReason int

const (
	_ LegacyCancelationReason = iota
	ExplicitlyCanceled
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
	RetryOf           int          `gae:"retry_of"`
	Status            LegacyStatus `gae:"status"`
	StatusChangedTime time.Time    `gae:"status_changed_time"`
	URL               string       `gae:"url,noindex"`
}
