// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dm

// Equals returns true iff the two Quest_IDs are equivalent.
func (q *Quest_ID) Equals(o *Quest_ID) bool {
	return q.Id == o.Id
}

// QuestID is a helper function to obtain the *Quest_ID from this Attempt_ID.
func (a *Attempt_ID) QuestID() *Quest_ID {
	return &Quest_ID{a.Quest}
}

// Equals returns true iff the two Attempt_IDs are equivalent.
func (a *Attempt_ID) Equals(o *Attempt_ID) bool {
	return a.Quest == o.Quest && a.Id == o.Id
}

// QuestID is a helper function to obtain the *Quest_ID from this Execution_ID.
func (e *Execution_ID) QuestID() *Quest_ID {
	return &Quest_ID{e.Quest}
}

// AttemptID is a helper function to obtain the *Attempt_ID from this
// Execution_ID.
func (e *Execution_ID) AttemptID() *Attempt_ID {
	return &Attempt_ID{e.Quest, e.Attempt}
}

// Equals returns true iff the two Execution_IDs are equivalent.
func (e *Execution_ID) Equals(o *Execution_ID) bool {
	return e.Quest == o.Quest && e.Attempt == o.Attempt && e.Id == o.Id
}
