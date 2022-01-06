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

package gerrit

import "fmt"

// Whom identifies one or a group of Gerrit users involved in the code review
// workflow.
type Whom int32

const (
	// Owner is the owner of the CL (user who uploaded the first patchset).
	Owner Whom = 1
	// Reviewers are the reviewers of the CL.
	Reviewers Whom = 2
	// CQVoters are the users who have voted on CQ label to trigger a CV Run.
	CQVoters Whom = 3
)

func (w Whom) String() string {
	switch w {
	case Owner:
		return "owner"
	case Reviewers:
		return "reviewers"
	case CQVoters:
		return "CQ label voters"
	default:
		return fmt.Sprintf("whom[%d]", w)
	}
}
