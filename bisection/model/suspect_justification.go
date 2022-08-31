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

import (
	"sort"
	"strings"
)

type JustificationType int64

const (
	JustificationType_UNSPECIFIED JustificationType = 0
	// If a commit touches a file in the failure log
	JustificationType_FAILURELOG JustificationType = 1
	// If a commit touches a file in the dependency
	JustificationType_DEPENDENCY JustificationType = 2
)

// SuspectJustification represents the heuristic analysis of a CL.
// It how likely the suspect is the real culprit and also the reason for suspecting.
type SuspectJustification struct {
	IsNonBlamable bool
	Items         []*SuspectJustificationItem
}

// SuspectJustificationItem represents one item of SuspectJustification
type SuspectJustificationItem struct {
	Score    int
	FilePath string
	Reason   string
	Type     JustificationType
}

func (justification *SuspectJustification) GetScore() int {
	score := 0
	dependencyScore := 0
	for _, item := range justification.Items {
		switch item.Type {
		case JustificationType_FAILURELOG:
			score += item.Score
		case JustificationType_DEPENDENCY:
			dependencyScore += item.Score
		default:
			// Do nothing
		}
	}
	// Maximum score a suspect can gain from dependency
	dependencyScoreThreshold := 9
	if dependencyScore > dependencyScoreThreshold {
		dependencyScore = dependencyScoreThreshold
	}
	return score + dependencyScore
}

func (justification *SuspectJustification) GetReasons() string {
	if justification.IsNonBlamable {
		return "The author is non-blamable"
	}
	reasons := make([]string, len(justification.Items))
	for i, item := range justification.Items {
		reasons[i] = item.Reason
	}
	return strings.Join(reasons, "\n")
}

func (justification *SuspectJustification) AddItem(score int, filePath string, reason string, justificationType JustificationType) {
	item := &SuspectJustificationItem{
		Score:    score,
		FilePath: filePath,
		Reason:   reason,
		Type:     justificationType,
	}
	if justification.Items == nil {
		justification.Items = []*SuspectJustificationItem{}
	}
	justification.Items = append(justification.Items, item)
}

// Sort sorts the items descendingly based on score
func (justification *SuspectJustification) Sort() {
	sort.Slice(justification.Items, func(i, j int) bool {
		return justification.Items[i].Score > justification.Items[j].Score
	})
}
