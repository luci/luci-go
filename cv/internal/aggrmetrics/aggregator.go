// Copyright 2021 The LUCI Authors.
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

package aggrmetrics

import (
	"context"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/tsmon/types"
)

type aggregator interface {
	// metrics must return of metrics set by the aggregator, which should be reset
	// after the tsmon flush.
	metrics() []types.Metric

	// prepare should load the data to be reported given the active LUCI projects,
	// and return the reportFunc to be called at the time of the next tsmon flush.
	//
	// It must be quick, i.e. it must finish within a few seconds.
	// If necessary, it may give away work to TQ tasks, which in turn save their
	// result into Datastore or Redis in order to be reported at a later time.
	//
	// The returned reportFunc must not block on any RPC -- it should update
	// the metrics using the data already available in RAM.
	prepare(ctx context.Context, activeProjects stringset.Set) (reportFunc, error)
}

// reportFunc updates metrics.
//
// reportFunc must be very fast and not block on any RPC.
// It should update the metrics using the data already available in RAM.
type reportFunc func(ctx context.Context)

func removeNils(in []reportFunc) []reportFunc {
	out := in[:0]
	for _, f := range in {
		if f != nil {
			out = append(out, f)
		}
	}
	return out
}
