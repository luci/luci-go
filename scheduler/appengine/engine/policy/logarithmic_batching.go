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

package policy

import (
	"math"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/scheduler/appengine/internal"
)

// LogarithmicBatchingPolicy instantiates new LOGARITHMIC_BATCHING policy
// function.
//
// It takes all pending triggers and collapses floor(log(base,N)) of them into
// one new invocation, deriving its properties from the most recent trigger
// alone.
func LogarithmicBatchingPolicy(maxConcurrentInvs, maxBatchSize int, logBase float64) (Func, error) {
	// We use 1.0001 to ensure that operation below returns a value small enough
	// to fit into int and to avoid numerical stability issues resulting from
	// very small values being approximated as 0 and resulting in
	// division-by-zero errors.
	if logBase < 1.0001 {
		return nil, errors.Reason("log_base should be more or equal than 1.0001").Err()
	}

	log := math.Log(logBase)
	return basePolicy(maxConcurrentInvs, maxBatchSize, func(triggers []*internal.Trigger) int {
		n := float64(len(triggers))
		target := math.Floor(math.Log(n) / log)
		return int(math.Min(math.Max(target, 1.0), n))
	})
}
