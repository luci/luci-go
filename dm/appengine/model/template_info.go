// Copyright 2016 The LUCI Authors.
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

package model

import (
	"sort"

	"github.com/xtgo/set"
	"go.chromium.org/luci/common/data/sortby"
	dm "go.chromium.org/luci/dm/api/service/v1"
)

// TemplateInfo is an ordered list of dm.Quest_TemplateSpec's
type TemplateInfo []dm.Quest_TemplateSpec

func (ti TemplateInfo) Len() int      { return len(ti) }
func (ti TemplateInfo) Swap(i, j int) { ti[i], ti[j] = ti[j], ti[i] }
func (ti TemplateInfo) Less(i, j int) bool {
	return sortby.Chain{
		func(i, j int) bool { return ti[i].Project < ti[j].Project },
		func(i, j int) bool { return ti[i].Ref < ti[j].Ref },
		func(i, j int) bool { return ti[i].Version < ti[j].Version },
		func(i, j int) bool { return ti[i].Name < ti[j].Name },
	}.Use(i, j)
}

// EqualsData returns true iff this TemplateInfo has the same content as the
// proto-style TemplateInfo. This assumes that `other` is sorted.
func (ti TemplateInfo) EqualsData(other []*dm.Quest_TemplateSpec) bool {
	if len(other) != len(ti) {
		return false
	}
	for i, me := range ti {
		if !me.Equals(other[i]) {
			return false
		}
	}
	return true
}

// Add adds ts to the TemplateInfo uniq'ly.
func (ti *TemplateInfo) Add(ts ...dm.Quest_TemplateSpec) {
	*ti = append(*ti, ts...)
	sort.Sort(*ti)
	*ti = (*ti)[:set.Uniq(*ti)]
}
