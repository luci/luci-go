// Copyright 2022 The LUCI Authors.
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

import "sort"

type HeuristicAnalysisResult struct {
	// A slice of possible culprit, sorted by score descendingly
	Items []*HeuristicAnalysisResultItem
}

type HeuristicAnalysisResultItem struct {
	Commit        string
	ReviewUrl     string
	ReviewTitle   string
	Justification *SuspectJustification
}

// AddItem adds a suspect to HeuristicAnalysisResult.
func (r *HeuristicAnalysisResult) AddItem(commit string, reviewUrl string, reviewTitle string, justification *SuspectJustification) {
	item := &HeuristicAnalysisResultItem{
		Commit:        commit,
		ReviewUrl:     reviewUrl,
		ReviewTitle:   reviewTitle,
		Justification: justification,
	}
	r.Items = append(r.Items, item)
}

// Sort items descendingly based on score (CLs with higher possibility to be
// culprit will come first).
func (r *HeuristicAnalysisResult) Sort() {
	sort.Slice(r.Items, func(i, j int) bool {
		return r.Items[i].Justification.GetScore() > r.Items[j].Justification.GetScore()
	})
}
