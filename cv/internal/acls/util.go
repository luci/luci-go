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

package acls

import (
	"fmt"
	"sort"
	"strings"

	"go.chromium.org/luci/cv/internal/changelist"
)

// CheckResult tells the result of an ACL check performed.
type CheckResult map[*changelist.CL]string

// OK returns true if the result indicates no failures. False, otherwise.
func (res CheckResult) OK() bool {
	return len(res) == 0
}

// Has tells whether CheckResult contains the provided CL.
func (res CheckResult) Has(cl *changelist.CL) bool {
	_, ok := res[cl]
	return ok
}

// Failure returns a failure message for a given RunCL.
//
// Returns an empty string, if the result was ok.
func (res CheckResult) Failure(cl *changelist.CL) string {
	if res.OK() {
		return ""
	}
	msg, ok := res[cl]
	if !ok {
		eids := make([]string, 0, len(res))
		for cl := range res {
			eids = append(eids, cl.ExternalID.MustURL())
		}
		sort.Strings(eids)

		var sb strings.Builder
		sb.WriteString(okButDueToOthers)
		for _, eid := range eids {
			sb.WriteString("\n  - ")
			sb.WriteString(eid)
		}
		return sb.String()
	}
	return msg
}

// FailuresSummary returns a summary of all the failures reported.
//
// Returns an empty string, if the result was ok.
func (res CheckResult) FailuresSummary() string {
	if res.OK() {
		return ""
	}
	msgs := make([]string, 0, len(res))
	for cl, msg := range res {
		msgs = append(msgs, fmt.Sprintf("* %s\n%s", cl.ExternalID.MustURL(), msg))
	}
	sort.Strings(msgs)

	var sb strings.Builder
	sb.WriteString(msgs[0])
	for _, msg := range msgs[1:] {
		sb.WriteString("\n\n")
		sb.WriteString(msg)
	}
	return sb.String()
}

// evalResult represents the result of an evaluation function.
//
// reason is the reason explaining why ok == false.
type evalResult struct {
	ok     bool
	reason string
}

var (
	yes = evalResult{ok: true}
	no  = evalResult{}
)

func noWithReason(r string) evalResult {
	return evalResult{reason: r}
}
