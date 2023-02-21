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

package prjpb

import (
	"go.chromium.org/luci/cv/internal/prjmanager/copyonwrite"
)

// COWPurgingCLs copy-on-write modifies PurgingCLs.
func (p *PState) COWPurgingCLs(m func(*PurgingCL) *PurgingCL, toAdd []*PurgingCL) ([]*PurgingCL, bool) {
	in := cowPurgingCLs(p.GetPurgingCls())
	out, updated := copyonwrite.Update(in, purgingCLModifier(m), cowPurgingCLs(toAdd))
	return []*PurgingCL(out.(cowPurgingCLs)), updated
}

func purgingCLModifier(f func(*PurgingCL) *PurgingCL) copyonwrite.Modifier {
	if f == nil {
		return nil
	}
	return func(v any) any {
		if v := f(v.(*PurgingCL)); v != nil {
			return v
		}
		return copyonwrite.Deletion
	}
}

type cowPurgingCLs []*PurgingCL

// It's important that PurgingCLs are always sorted.
var _ copyonwrite.SortedSlice = cowPurgingCLs(nil)

func (c cowPurgingCLs) At(index int) any {
	return c[index]
}

func (c cowPurgingCLs) Append(v any) copyonwrite.Slice {
	return append(c, v.(*PurgingCL))
}

func (c cowPurgingCLs) CloneShallow(length int, capacity int) copyonwrite.Slice {
	r := make(cowPurgingCLs, length, capacity)
	copy(r, c[:length])
	return r
}

func (c cowPurgingCLs) LessElements(a any, b any) bool {
	return a.(*PurgingCL).GetClid() < b.(*PurgingCL).GetClid()
}

func (c cowPurgingCLs) Len() int               { return len(c) }
func (c cowPurgingCLs) Less(i int, j int) bool { return c[i].GetClid() < c[j].GetClid() }
func (c cowPurgingCLs) Swap(i int, j int)      { c[i], c[j] = c[j], c[i] }
