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

	. "github.com/smartystreets/goconvey/convey"
)

func TestTailLikelihoods(t *testing.T) {
	Convey(`TailLikelihoods invariants`, t, func() {
		// Check that likelihoods are in strictly increasing order.
		var lastLikelihood float64
		for _, likelihood := range TailLikelihoods {
			So(likelihood, ShouldBeGreaterThan, lastLikelihood)
			lastLikelihood = likelihood
		}

		// Check that likelihoods are symmetric around the center.
		for i, likelihood := range TailLikelihoods {
			So(likelihood, ShouldAlmostEqual, 1.0-TailLikelihoods[len(TailLikelihoods)-1-i])
		}
	})
}

func TestConfidenceInterval(t *testing.T) {
	Convey(`ConfidenceInterval`, t, func() {
		d := SimpleDistribution(100, 10)
		min, max := d.ConfidenceInterval(0.99)
		So(min, ShouldEqual, 90)
		So(max, ShouldEqual, 110)
	})
}
