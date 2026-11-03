// Copyright 2026 The LUCI Authors.
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

package cfgcache

import (
	"cmp"
	"iter"
	"slices"

	"go.chromium.org/luci/config"
)

// flatSnapshot indicates a bunch of configs which originated from the same
// config.Set.
//
// These may, or may not, actually contain `Content`, but will always have
// `Meta`.
//
// Sorted by Path.
type flatSnapshot []*config.Config

// flatSnapshotFromMap takes loaded configs from a single config.Set (e.g. as
// returned by GetConfigs) and returns them as a flatSnapshot.
func flatSnapshotFromMap(snap map[string]config.Config) flatSnapshot {
	flattened := make(flatSnapshot, 0, len(snap))
	for _, val := range snap {
		flattened = append(flattened, &val)
	}
	slices.SortFunc(flattened, func(a, b *config.Config) int {
		return cmp.Compare(a.Path, b.Path)
	})
	return flattened
}

// get retrieves the config whose Path is `path`, or `nil` if it doesn't exist
// in this snapshot.
func (f flatSnapshot) get(path string) *config.Config {
	idx, ok := slices.BinarySearchFunc(f, path, func(cfg *config.Config, targ string) int {
		return cmp.Compare(cfg.Path, targ)
	})
	if !ok {
		return nil
	}
	return f[idx]
}

// paths returns the list of paths in this snapshot.
func (f flatSnapshot) paths() []string {
	ret := make([]string, len(f))
	for i, cfg := range f {
		ret[i] = cfg.Path
	}
	return ret
}

// maps config.Set to a snapshot of config data in that config.Set.
type loadedConfig map[config.Set]flatSnapshot

// serviceConfigs returns an iterator over the (singular) service config.Set
// matching serviceName.
//
// This only makes sense as a counterpart to [loadedConfig.projectConfigs].
func (l loadedConfig) serviceConfigs(serviceName string) iter.Seq2[config.Set, flatSnapshot] {
	return func(yield func(config.Set, flatSnapshot) bool) {
		cs := config.MustServiceSet(serviceName)
		yield(cs, l[cs])
	}
}

// projectConfigs is an iterator which yields all project configs in this
// loadedConfig.
func (l loadedConfig) projectConfigs(yield func(config.Set, flatSnapshot) bool) {
	for cs, cfg := range l {
		if cs.Project() != "" {
			if !yield(cs, cfg) {
				return
			}
		}
	}
}

// loadedConfigFromConfigs takes a set of unordered configs across multiple
// config sets (e.g. as returned by GetProjectConfigs), and organizes them into
// a loadedConfig.
func loadedConfigFromConfigs(configs []config.Config) loadedConfig {
	byCfgSet := make(loadedConfig, len(configs))
	for _, cfg := range configs {
		byCfgSet[cfg.ConfigSet] = append(byCfgSet[cfg.ConfigSet], &cfg)
	}

	for cs := range byCfgSet {
		slices.SortFunc(byCfgSet[cs], func(a, b *config.Config) int {
			return cmp.Compare(a.Path, b.Path)
		})
	}
	return byCfgSet
}
