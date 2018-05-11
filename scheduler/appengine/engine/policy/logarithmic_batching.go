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
// It takes all pending triggers and collapses log_k N of them into one new
// invocation, deriving its properties from the most recent trigger alone.
func LogarithmicBatchingPolicy(maxConcurrentInvs, maxBatchSize, logBase int) (Func, error) {
	if logBase <= 1 {
		return nil, errors.Reason("log_base should be at least 2").Err()
	}

	log := math.Log(logBase)
	return basePolicy(maxConcurrentInvs, maxBatchSize, func(triggers []*internal.Trigger) int {
		return int(math.Max(math.Log(float64(len(triggers)))/log, 1.0))
	})
}
