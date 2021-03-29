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

package cvtesting

import (
	"sort"
	"sync"
	"time"
)

// DispatchRecorder records dispatches in memory.
type DispatchRecorder struct {
	m       sync.Mutex
	targets map[string][]time.Time
}

// Dispatch records a dispatch.
func (d *DispatchRecorder) Dispatch(target string, eta time.Time) {
	d.m.Lock()
	if d.targets == nil {
		d.targets = make(map[string][]time.Time, 1)
	}
	d.targets[target] = append(d.targets[target], eta.UTC())
	d.m.Unlock()
}

// Clear clears all recorded dispatches.
func (d *DispatchRecorder) Clear() {
	d.m.Lock()
	d.targets = nil
	d.m.Unlock()
}

// Targets returns sorted list of targets.
func (d *DispatchRecorder) Targets() []string {
	return d.sortedTargets(false)
}

// PopTargets returns sorted list of targets and clears the state.
func (d *DispatchRecorder) PopTargets() []string {
	return d.sortedTargets(true)
}

func (d *DispatchRecorder) sortedTargets(clear bool) []string {
	d.m.Lock()
	ps := make([]string, 0, len(d.targets))
	for p := range d.targets {
		ps = append(ps, p)
	}
	if clear {
		d.targets = nil
	}
	d.m.Unlock()
	sort.Strings(ps)
	return ps
}

// ETAsOf returns sorted distinct ETAs for dispatches of the given target.
func (d *DispatchRecorder) ETAsOf(target string) []time.Time {
	var out []time.Time
	d.m.Lock()
	out = append(out, d.targets[target]...) // copy
	d.m.Unlock()

	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool { return !out[i].After(out[j]) })
	unique := out[:1]
	for _, t := range out[1:] {
		if t != unique[len(unique)-1] {
			unique = append(unique, t)
		}
	}
	return unique
}
