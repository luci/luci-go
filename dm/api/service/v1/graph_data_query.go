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

// ToQuery generates a new GraphQuery.
//
// This generates a GraphQuery that queries for any Attempts which are marked as
// Partial in the current GraphData.
func (g *GraphData) ToQuery() (ret *GraphQuery) {
	partials := map[string][]uint32{}
	for qid, qst := range g.Quests {
		// TODO(iannucci): handle q.Partial explicitly?
		for aid, atmpt := range qst.Attempts {
			if atmpt.Partial.Any() {
				partials[qid] = append(partials[qid], aid)
			}
		}
	}

	if len(partials) > 0 {
		ret = &GraphQuery{AttemptList: NewAttemptList(partials)}
	}
	return
}
