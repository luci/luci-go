// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dm

// AttemptListQuery returns a new GraphQuery for the given AttemptList.
func AttemptListQuery(fanout *AttemptList) *GraphQuery {
	return &GraphQuery{
		Query: &GraphQuery_AttemptList_{
			AttemptList: &GraphQuery_AttemptList{
				Attempt: fanout,
			},
		},
	}
}

// AttemptListQueryL returns a new GraphQuery for the given AttemptList
// literal.
func AttemptListQueryL(fanout map[string][]uint32) *GraphQuery {
	return &GraphQuery{
		Query: &GraphQuery_AttemptList_{
			AttemptList: &GraphQuery_AttemptList{
				Attempt: NewAttemptList(fanout),
			},
		},
	}
}

// AttemptRangeQuery returns a new GraphQuery for the given AttemptRange
// specification.
func AttemptRangeQuery(quest string, low, high uint32) *GraphQuery {
	return &GraphQuery{
		Query: &GraphQuery_AttemptRange_{
			AttemptRange: &GraphQuery_AttemptRange{quest, low, high},
		},
	}
}
