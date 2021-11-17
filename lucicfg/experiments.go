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

package lucicfg

import (
	"fmt"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"

	"go.starlark.net/starlark"
)

// experiments holds a set of registered experiment IDs and enabled ones.
type experiments struct {
	all     stringset.Set
	enabled stringset.Set
}

// Register adds an experiment ID to the set of known experiments.
//
// Does nothing if such ID has already been registered.
func (exp *experiments) Register(id string) {
	if exp.all == nil {
		exp.all = stringset.New(1)
	}
	exp.all.Add(id)
}

// Registered returns a sorted list of all registered experiments.
func (exp *experiments) Registered() []string {
	return exp.all.ToSortedSlice()
}

// Enable adds an experiment ID to the set of enabled experiments.
//
// Always succeeds, but returns false if the experiment ID hasn't been
// registered before.
func (exp *experiments) Enable(id string) bool {
	if exp.enabled == nil {
		exp.enabled = stringset.New(1)
	}
	exp.enabled.Add(id)
	return exp.all.Has(id)
}

// IsEnabled returns true if an experiment has been enabled already.
func (exp *experiments) IsEnabled(id string) bool {
	return exp.enabled.Has(id)
}

func init() {
	// enable_experiment is used by lucicfg.enable_experiment in lucicfg.star.
	declNative("enable_experiment", func(call nativeCall) (starlark.Value, error) {
		var id starlark.String
		if err := call.unpack(1, &id); err != nil {
			return nil, err
		}
		if expID := id.GoString(); !call.State.experiments.Enable(expID) {
			help := "there are no experiments available"
			if all := call.State.experiments.Registered(); len(all) != 0 {
				quoted := make([]string, len(all))
				for i, s := range all {
					quoted[i] = fmt.Sprintf("%q", s)
				}
				help = "available experiments: " + strings.Join(quoted, ", ")
			}
			logging.Warningf(call.Ctx, "enable_experiment: unknown experiment %q (%s). "+
				"It is possible the experiment was retired already, consider removing this call to stop the warning.", expID, help)
		}
		return starlark.None, nil
	})

	// register_experiment is used in experiments.star.
	declNative("register_experiment", func(call nativeCall) (starlark.Value, error) {
		var id starlark.String
		if err := call.unpack(1, &id); err != nil {
			return nil, err
		}
		call.State.experiments.Register(id.GoString())
		return starlark.None, nil
	})

	// is_experiment_enabled is used in experiments.star.
	declNative("is_experiment_enabled", func(call nativeCall) (starlark.Value, error) {
		var id starlark.String
		if err := call.unpack(1, &id); err != nil {
			return nil, err
		}
		return starlark.Bool(call.State.experiments.IsEnabled(id.GoString())), nil
	})
}
