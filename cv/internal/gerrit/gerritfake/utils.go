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

package gerritfake

import (
	"github.com/smarty/assertions"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

// ResetVotes resets all non-0 votes of the given labels to 0.
func ResetVotes(ci *gerritpb.ChangeInfo, labels ...string) {
	for _, label := range labels {
		info := ci.Labels[label]
		if info == nil {
			continue
		}
		for _, vote := range info.All {
			if vote.Value != 0 {
				vote.Value = 0
				vote.Date = nil
			}
		}
	}
}

// NonZeroVotes returns all non-zero votes for the provided label.
//
// Return nil slice if label doesn't exist.
func NonZeroVotes(ci *gerritpb.ChangeInfo, label string) []*gerritpb.ApprovalInfo {
	l, ok := ci.GetLabels()[label]
	if !ok {
		return nil
	}
	ret := make([]*gerritpb.ApprovalInfo, 0, len(l.GetAll()))
	for _, ai := range l.GetAll() {
		if ai.GetValue() != 0 {
			ret = append(ret, ai)
		}
	}
	return ret
}

// LastMessage returns the last message from the Gerrit Change.
//
// Return nil if there are no messages.
func LastMessage(ci *gerritpb.ChangeInfo) *gerritpb.ChangeMessageInfo {
	ms := ci.GetMessages()
	if l := len(ms); l > 0 {
		return ms[l-1]
	}
	return nil
}

// ShouldLastMessageContain asserts the last posted message on a ChangeInfo
// contains the expected substring.
func ShouldLastMessageContain(actual any, oneSubstring ...any) string {
	if len(oneSubstring) != 1 {
		panic("exactly 1 substring required")
	}
	if diff := assertions.ShouldHaveSameTypeAs(oneSubstring[0], string("")); diff != "" {
		return diff
	}
	if diff := assertions.ShouldHaveSameTypeAs(actual, (*gerritpb.ChangeInfo)(nil)); diff != "" {
		return diff
	}
	ci := actual.(*gerritpb.ChangeInfo)
	expected := oneSubstring[0].(string)

	last := LastMessage(ci)
	if last == nil {
		return "ChangeInfo has no messages"
	}
	return assertions.ShouldContainSubstring(last.GetMessage(), expected)
}
