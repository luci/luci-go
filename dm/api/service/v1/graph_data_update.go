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

package dm

import (
	"github.com/xtgo/set"
)

// UpdateWith updates this GraphData with all of the data contained in other.
// Assumes that any mutable data in other is more up-to-date than the data in g.
func (g *GraphData) UpdateWith(other *GraphData) {
	for qid, q := range other.Quests {
		if curQ, ok := g.Quests[qid]; !ok {
			g.Quests[qid] = q
		} else {
			curQ.UpdateWith(q)
		}
	}
}

// UpdateWith updates this Quest with data from other.
func (q *Quest) UpdateWith(o *Quest) {
	if q.Data == nil {
		q.Data = o.Data
		q.Partial = false
	} else if !o.Partial {
		q.Data.UpdateWith(o.Data)
		q.Partial = false
	}
	if q.Id == nil {
		q.Id = o.Id
	}
	for aid, a := range o.Attempts {
		if curA, ok := q.Attempts[aid]; !ok {
			q.Attempts[aid] = a
		} else {
			curA.UpdateWith(a)
		}
	}
}

// UpdateWith updates this Quest_Data with data from other.
func (q *Quest_Data) UpdateWith(other *Quest_Data) {
	pivot := len(q.BuiltBy)
	q.BuiltBy = append(q.BuiltBy, other.BuiltBy...)
	q.BuiltBy = q.BuiltBy[:set.Union(QuestTemplateSpecs(q.BuiltBy), pivot)]
}

// UpdateWith updates this Attempt with data from other.
func (a *Attempt) UpdateWith(other *Attempt) {
	if a.Partial == nil {
		a.Partial = &Attempt_Partial{}
	}

	if a.Data == nil {
		a.Data = other.Data
		a.Partial.Data = false
	} else if other.Partial != nil && !other.Partial.Data {
		a.Data.UpdateWith(other.Data)
		a.Partial.Data = false
	}

	if a.Id == nil {
		a.Id = other.Id
	}

	for eid, e := range other.Executions {
		if curE, ok := a.Executions[eid]; !ok {
			a.Executions[eid] = e
		} else {
			curE.UpdateWith(e)
		}
	}

	if a.FwdDeps == nil {
		a.FwdDeps = other.FwdDeps
	} else {
		a.FwdDeps.UpdateWith(other.FwdDeps)
	}

	if a.BackDeps == nil {
		a.BackDeps = other.BackDeps
	} else {
		a.BackDeps.UpdateWith(other.BackDeps)
	}
}

// UpdateWith updates this Attempt_Data with data from other.
func (a *Attempt_Data) UpdateWith(other *Attempt_Data) {
	if a.Created == nil {
		a.Created = other.Created
	}
	if other.Modified != nil {
		a.Modified = other.Modified
	}
	a.NumExecutions = other.NumExecutions
	if other.AttemptType != nil {
		a.AttemptType = other.AttemptType
	}
}

// UpdateWith updates this Execution with data from other.
func (e *Execution) UpdateWith(other *Execution) {
	if e.Id == nil {
		e.Id = other.Id
	}
	if e.Data == nil {
		e.Data = other.Data
		e.Partial = false
	} else if !other.Partial {
		e.Data.UpdateWith(other.Data)
		e.Partial = false
	}
}

// UpdateWith updates this Execution_Data with data from other.
func (e *Execution_Data) UpdateWith(other *Execution_Data) {
	if e.Created == nil {
		e.Created = other.Created
	}
	if other.Modified != nil {
		e.Modified = other.Modified
	}
	if other.DistributorInfo != nil {
		e.DistributorInfo = other.DistributorInfo
	}
	if other.ExecutionType != nil {
		e.ExecutionType = other.ExecutionType
	}
}
