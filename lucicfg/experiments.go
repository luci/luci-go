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
	"sort"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
)

// experiments holds a set of registered experiment IDs and enabled ones.
type experiments struct {
	all        map[string]version // name => lucicfg version to auto-enable on
	enabled    stringset.Set
	lucicfgVer version // max of versions passed to lucicfg.check_version(...)
}

// Register adds an experiment ID to the set of known experiments.
func (exp *experiments) Register(id string, minVer starlark.Tuple) {
	if exp.all == nil {
		exp.all = make(map[string]version, 1)
	}

	ver := version{minVer}
	exp.all[id] = ver

	// Auto-enable based on version passed to lucicfg.check_version(...).
	if exp.lucicfgVer.isSet() && ver.isSet() && exp.lucicfgVer.greaterOrEq(ver) {
		exp.Enable(id)
	}
}

// Registered returns a sorted list of all registered experiments.
func (exp *experiments) Registered() []string {
	names := make([]string, 0, len(exp.all))
	for name := range exp.all {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
	_, known := exp.all[id]
	return known
}

// IsEnabled returns true if an experiment has been enabled already.
func (exp *experiments) IsEnabled(id string) bool {
	return exp.enabled.Has(id)
}

// setMinVersion is called from lucicfg.check_version(...).
//
// Auto-enables eligible experiments.
func (exp *experiments) setMinVersion(minVer starlark.Tuple) {
	ver := version{minVer}
	if !ver.isSet() {
		panic(fmt.Sprintf("empty version passed to setMinVersion: %v", minVer))
	}
	if exp.lucicfgVer.isSet() && exp.lucicfgVer.greaterOrEq(ver) {
		return // already checked with more recent version
	}
	exp.lucicfgVer = ver

	// Auto-enable experiments based on their enable_on_min_version.
	for id, min := range exp.all {
		if min.isSet() && exp.lucicfgVer.greaterOrEq(min) {
			exp.Enable(id)
		}
	}
}

type version struct {
	tup starlark.Tuple // either () or (major, minor, revision)
}

func (v version) isSet() bool {
	return len(v.tup) != 0
}

func (v version) greaterOrEq(another version) bool {
	yes, err := starlark.Compare(syntax.GE, v.tup, another.tup)
	if err != nil {
		panic(fmt.Sprintf("comparing tuples should succeed, got: %s", err))
	}
	return yes
}

func init() {
	// set_min_version_for_experiments is called from lucicfg.check_version.
	declNative("set_min_version_for_experiments", func(call nativeCall) (starlark.Value, error) {
		var ver starlark.Tuple
		if err := call.unpack(1, &ver); err != nil {
			return nil, err
		}
		call.State.experiments.setMinVersion(ver)
		return starlark.None, nil
	})

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
		var minVer starlark.Tuple
		if err := call.unpack(1, &id, &minVer); err != nil {
			return nil, err
		}
		call.State.experiments.Register(id.GoString(), minVer)
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

	// list_enabled_experiments lists experiments enabled via enable_experiment.
	//
	// Lists all experiments passed to enable_experiment(...), even ones that
	// aren't registered anymore. Also includes all experiments auto-enabled via
	// `enable_on_min_version` mechanism.
	//
	// This list ends up in `lucicfg {...}` section of project.cfg. Listing *all*
	// experiments there is useful to figure out what LUCI projects enable retired
	// experiments.
	declNative("list_enabled_experiments", func(call nativeCall) (starlark.Value, error) {
		if err := call.unpack(0); err != nil {
			return nil, err
		}
		exps := make([]starlark.Value, 0, call.State.experiments.enabled.Len())
		for _, exp := range call.State.experiments.enabled.ToSortedSlice() {
			exps = append(exps, starlark.String(exp))
		}
		return starlark.NewList(exps), nil
	})
}
