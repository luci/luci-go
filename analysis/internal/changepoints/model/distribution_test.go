// Copyright 2024 The LUCI Authors.
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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTailLikelihoods(t *testing.T) {
	ftt.Run(`TailLikelihoods invariants`, t, func(t *ftt.Test) {
		// Check that likelihoods are in strictly increasing order.
		var lastLikelihood float64
		for _, likelihood := range TailLikelihoods {
			assert.Loosely(t, likelihood, should.BeGreaterThan(lastLikelihood))
			lastLikelihood = likelihood
		}

		// Check that likelihoods are symmetric around the center.
		for i, likelihood := range TailLikelihoods {
			assert.Loosely(t, likelihood, should.AlmostEqual(1.0-TailLikelihoods[len(TailLikelihoods)-1-i]))
		}
	})
}

func TestConfidenceInterval(t *testing.T) {
	ftt.Run(`ConfidenceInterval`, t, func(t *ftt.Test) {
		d := SimpleDistribution(100, 10)
		min, max := d.ConfidenceInterval(0.99)
		assert.Loosely(t, min, should.Equal(90))
		assert.Loosely(t, max, should.Equal(110))
	})
}
