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
	"time"

	"go.chromium.org/luci/cv/internal/prjmanager/copyonwrite"
)

// MaxTriggeringCLsDuration limits the time that a TQ task has to execute
// a given TrigggeringCL task.
//
// Once the deadline is exceeded, PM will will remove the task from PStat
// to retriage the CL and see if it has to reschedule another TriggeringCL
// for the CL (or its deps).
const MaxTriggeringCLsDuration = 8 * time.Minute

// COWTriggeringCLs copy-on-write modifies TriggeringCLs.
func (p *PState) COWTriggeringCLs(m func(*TriggeringCL) *TriggeringCL, toAdd []*TriggeringCL) ([]*TriggeringCL, bool) {
	in := cowTriggeringCLs(p.GetTriggeringCls())
	out, updated := copyonwrite.Update(in, triggerCLModifier(m), cowTriggeringCLs(toAdd))
	return []*TriggeringCL(out.(cowTriggeringCLs)), updated
}

func triggerCLModifier(f func(*TriggeringCL) *TriggeringCL) copyonwrite.Modifier {
	if f == nil {
		return nil
	}
	return func(v any) any {
		if v := f(v.(*TriggeringCL)); v != nil {
			return v
		}
		return copyonwrite.Deletion
	}
}

type cowTriggeringCLs []*TriggeringCL

// It's important that TriggeringCLs are always sorted.
var _ copyonwrite.SortedSlice = cowTriggeringCLs(nil)

func (c cowTriggeringCLs) At(index int) any {
	return c[index]
}

func (c cowTriggeringCLs) Append(v any) copyonwrite.Slice {
	return append(c, v.(*TriggeringCL))
}

func (c cowTriggeringCLs) CloneShallow(length int, capacity int) copyonwrite.Slice {
	r := make(cowTriggeringCLs, length, capacity)
	copy(r, c[:length])
	return r
}

func (c cowTriggeringCLs) LessElements(a any, b any) bool {
	return a.(*TriggeringCL).GetClid() < b.(*TriggeringCL).GetClid()
}

func (c cowTriggeringCLs) Len() int               { return len(c) }
func (c cowTriggeringCLs) Less(i int, j int) bool { return c[i].GetClid() < c[j].GetClid() }
func (c cowTriggeringCLs) Swap(i int, j int)      { c[i], c[j] = c[j], c[i] }
