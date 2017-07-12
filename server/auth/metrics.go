// Copyright 2015 The LUCI Authors.
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

package auth

import (
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/tsmon/distribution"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/metric"
	"github.com/luci/luci-go/common/tsmon/types"
)

var (
	authenticateDuration = metric.NewCumulativeDistribution(
		"luci/auth/methods/authenticate",
		"Distribution of 'Authenticate' call durations per result.",
		&types.MetricMetadata{Units: types.Microseconds},
		distribution.DefaultBucketer,
		field.String("result"))

	mintDelegationTokenDuration = metric.NewCumulativeDistribution(
		"luci/auth/methods/mint_delegation_token",
		"Distribution of 'MintDelegationToken' call durations per result.",
		&types.MetricMetadata{Units: types.Microseconds},
		distribution.DefaultBucketer,
		field.String("result"))

	mintAccessTokenDuration = metric.NewCumulativeDistribution(
		"luci/auth/methods/mint_access_token",
		"Distribution of 'MintAccessTokenForServiceAccount' call durations per result.",
		&types.MetricMetadata{Units: types.Microseconds},
		distribution.DefaultBucketer,
		field.String("result"))
)

func durationReporter(c context.Context, m metric.CumulativeDistribution) func(error, string) {
	startTs := clock.Now(c)
	return func(err error, result string) {
		// We should report context errors as such. It doesn't matter at which stage
		// the deadline happens, thus we ignore 'result' if seeing a context error.
		switch errors.Unwrap(err) {
		case context.DeadlineExceeded:
			result = "CONTEXT_DEADLINE_EXCEEDED"
		case context.Canceled:
			result = "CONTEXT_CANCELED"
		}
		m.Add(c, float64(clock.Since(c, startTs).Nanoseconds()/1000), result)
	}
}
