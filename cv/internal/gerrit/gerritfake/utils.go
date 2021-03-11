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
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

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
