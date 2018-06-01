// Copyright 2015 The LUCI Authors.
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

// Equals returns true iff the two Quest_IDs are equivalent.
func (q *Quest_ID) Equals(o *Quest_ID) bool {
	return q.Id == o.Id
}

// QuestID is a helper function to obtain the *Quest_ID from this Attempt_ID.
func (a *Attempt_ID) QuestID() *Quest_ID {
	return &Quest_ID{Id: a.Quest}
}

// Equals returns true iff the two Attempt_IDs are equivalent.
func (a *Attempt_ID) Equals(o *Attempt_ID) bool {
	return a.Quest == o.Quest && a.Id == o.Id
}

// Execution returns an Execution_ID for this Attempt.
func (a *Attempt_ID) Execution(eid uint32) *Execution_ID {
	return &Execution_ID{Quest: a.Quest, Attempt: a.Id, Id: eid}
}

// QuestID is a helper function to obtain the *Quest_ID from this Execution_ID.
func (e *Execution_ID) QuestID() *Quest_ID {
	return &Quest_ID{Id: e.Quest}
}

// AttemptID is a helper function to obtain the *Attempt_ID from this
// Execution_ID.
func (e *Execution_ID) AttemptID() *Attempt_ID {
	return &Attempt_ID{Quest: e.Quest, Id: e.Attempt}
}

// Equals returns true iff the two Execution_IDs are equivalent.
func (e *Execution_ID) Equals(o *Execution_ID) bool {
	return e.Quest == o.Quest && e.Attempt == o.Attempt && e.Id == o.Id
}
