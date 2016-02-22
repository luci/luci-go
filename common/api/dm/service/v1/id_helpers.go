// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dm

// QuestID is a helper function to obtain the *Quest_ID from this Attempt_ID.
func (a *Attempt_ID) QuestID() *Quest_ID {
	return &Quest_ID{a.Quest}
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
