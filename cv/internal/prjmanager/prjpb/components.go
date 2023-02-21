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

// CloneShallow creates a new shallow clone.
func (c *Component) CloneShallow() *Component {
	return &Component{
		Clids:          c.GetClids(),
		Pruns:          c.GetPruns(),
		DecisionTime:   c.GetDecisionTime(),
		TriageRequired: c.GetTriageRequired(),
	}
}

// COWComponents copy-on-write modifies components.
func (p *PState) COWComponents(m func(*Component) *Component, toAdd []*Component) ([]*Component, bool) {
	var mf copyonwrite.Modifier
	if m != nil {
		mf = func(v any) any {
			if v := m(v.(*Component)); v != nil {
				return v
			}
			return copyonwrite.Deletion
		}
	}
	in := cowComponents(p.GetComponents())
	out, updated := copyonwrite.Update(in, mf, cowComponents(toAdd))
	return []*Component(out.(cowComponents)), updated
}

type cowComponents []*Component

func (c cowComponents) CloneShallow(length int, capacity int) copyonwrite.Slice {
	r := make(cowComponents, length, capacity)
	copy(r, c[:length])
	return r
}

func (c cowComponents) Append(v any) copyonwrite.Slice {
	return append(c, v.(*Component))
}

func (c cowComponents) Len() int {
	return len(c)
}

func (c cowComponents) At(index int) any {
	return c[index]
}
