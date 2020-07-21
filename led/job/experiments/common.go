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

package experiments

import (
	"context"

	"go.chromium.org/luci/common/errors"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	swarmingpb "go.chromium.org/luci/swarming/proto/api"
)

var knownExperiments = map[string]Experiment{}

// Experiment mutates `task` based on `b`.
type Experiment func(ctx context.Context, b *bbpb.Build, task *swarmingpb.TaskRequest) error

// Register registers a known experiment given its key and implementation.
func Register(key string, exp Experiment) {
	knownExperiments[key] = exp
}

// Apply mutates `task` based on known experiments in b.Input.Experiments.
func Apply(ctx context.Context, b *bbpb.Build, task *swarmingpb.TaskRequest) error {
	for _, name := range b.GetInput().GetExperiments() {
		if exp, ok := knownExperiments[name]; ok {
			if err := exp(ctx, b, task); err != nil {
				return errors.Annotate(err, "experiment %q", name).Err()
			}
		}
	}
	return nil
}
