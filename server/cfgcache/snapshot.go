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

import "go.chromium.org/luci/config"

type singleFileEntry struct {
	resolvedPath string
	val          any
}

// Snapshot holds the unmarshaled configuration files for a single
// [config.Set].
type Snapshot struct {
	configSet   config.Set
	singleFiles map[File]singleFileEntry
	regexFiles  map[File]any
}

func newSnapshot(cs config.Set) *Snapshot {
	return &Snapshot{cs, map[File]singleFileEntry{}, map[File]any{}}
}

// ConfigSet returns the concrete config set (e.g. "services/my-app" or
// "projects/chromium") that this snapshot belongs to.
func (s *Snapshot) ConfigSet() config.Set {
	return s.configSet
}
