// Copyright 2023 The LUCI Authors.
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

package prjpb

import (
	"sort"
	"time"

	"go.chromium.org/luci/cv/internal/prjmanager/copyonwrite"
)

// MaxTriggeringCLDepsDuration limits the time that a TQ task has to execute
// a given TrigggeringCLDesp task.
//
// Once the deadline is exceeded, PM will will remove the task from PStat
// to retriage the CL and see if it has to reschedule another TriggeringCLDeps
// for the CL with its deps.
const MaxTriggeringCLDepsDuration = 8 * time.Minute

// COWTriggeringCLDeps copy-on-write modifies TriggeringCLDeps.
func (p *PState) COWTriggeringCLDeps(m func(*TriggeringCLDeps) *TriggeringCLDeps, toAdd []*TriggeringCLDeps) ([]*TriggeringCLDeps, bool) {
	in := cowTriggeringCLDeps(p.GetTriggeringClDeps())
	out, updated := copyonwrite.Update(in, triggerCLDepsModifier(m), cowTriggeringCLDeps(toAdd))
	return []*TriggeringCLDeps(out.(cowTriggeringCLDeps)), updated
}

// GetTriggeringCLDeps returns the TriggeringCLDeps of a given origin clid.
//
// Returns nil, if not found.
func (p *PState) GetTriggeringCLDeps(clid int64) *TriggeringCLDeps {
	items := p.GetTriggeringClDeps()
	idx := sort.Search(len(items), func(i int) bool {
		return items[i].GetOriginClid() >= clid
	})
	if idx >= len(items) || items[idx].GetOriginClid() != clid {
		return nil
	}
	return items[idx]
}

func triggerCLDepsModifier(f func(*TriggeringCLDeps) *TriggeringCLDeps) copyonwrite.Modifier {
	if f == nil {
		return nil
	}
	return func(v any) any {
		if v := f(v.(*TriggeringCLDeps)); v != nil {
			return v
		}
		return copyonwrite.Deletion
	}
}

type cowTriggeringCLDeps []*TriggeringCLDeps

// It's important that TriggeringCLDeps are always sorted.
var _ copyonwrite.SortedSlice = cowTriggeringCLDeps(nil)

func (c cowTriggeringCLDeps) At(index int) any {
	return c[index]
}

func (c cowTriggeringCLDeps) Append(v any) copyonwrite.Slice {
	return append(c, v.(*TriggeringCLDeps))
}

func (c cowTriggeringCLDeps) CloneShallow(length int, capacity int) copyonwrite.Slice {
	r := make(cowTriggeringCLDeps, length, capacity)
	copy(r, c[:length])
	return r
}

func (c cowTriggeringCLDeps) LessElements(a any, b any) bool {
	return a.(*TriggeringCLDeps).GetOriginClid() < b.(*TriggeringCLDeps).GetOriginClid()
}

func (c cowTriggeringCLDeps) Len() int {
	return len(c)
}

func (c cowTriggeringCLDeps) Less(i int, j int) bool {
	return c[i].GetOriginClid() < c[j].GetOriginClid()
}
func (c cowTriggeringCLDeps) Swap(i int, j int) {
	c[i], c[j] = c[j], c[i]
}
