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

package userhtml

import (
	"fmt"
	"strings"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
)

// StringifySubmissionSuccesses composes a message indicating the urls of the CLs
// that were just submitted, and the number of pending CLs.
func StringifySubmissionSuccesses(clidToURL map[common.CLID]string, v *run.LogEntry_CLSubmitted, all int64) string {
	var sb strings.Builder
	currentlySubmitted := common.MakeCLIDs(v.NewlySubmittedCls...)
	switch len(currentlySubmitted) {
	case 0:
		sb.WriteString("No CLs were submitted successfully")
	case 1:
		_, _ = fmt.Fprintf(&sb, "CL %s was submitted successfully", clidToURL[currentlySubmitted[0]])
	default:
		_, _ = fmt.Fprintf(&sb, "%d CLs were submitted successfully:", len(currentlySubmitted))
		for _, clid := range currentlySubmitted {
			_, _ = fmt.Fprintf(&sb, "\n  * %s", clidToURL[clid])
		}
	}
	if left := all - v.TotalSubmitted; left > 0 {
		_, _ = fmt.Fprintf(&sb, "\n%d out of %d CLs are still pending", all-v.TotalSubmitted, all)
	}
	return sb.String()
}

// StringifySubmissionFailureReason makes a human-readable message detailing
// the reason for the failure of this submission.
func StringifySubmissionFailureReason(clidToURL map[common.CLID]string, sc *eventpb.SubmissionCompleted) string {
	var sb strings.Builder
	if sc.GetFailureReason() != nil {
		switch sc.GetFailureReason().(type) {
		case *eventpb.SubmissionCompleted_Timeout:
			return "Timeout"
		case *eventpb.SubmissionCompleted_ClFailures:
			msg, err := sFormatCLSubmissionFailures(clidToURL, sc.GetClFailures())
			if err != nil {
				panic(err)
			}
			sb.WriteString(msg)
		}
	}
	return sb.String()
}

func listify(cls []string) string {
	var sb strings.Builder
	for i, v := range cls {
		if i == len(cls)-1 && len(cls) > 1 {
			sb.WriteString(" and ")
		} else if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(v)
	}
	sb.WriteString(" ")
	return sb.String()
}

// sFormatCLSubmissionFailures returns a string with the messages for cl submission failures.
func sFormatCLSubmissionFailures(clidToURL map[common.CLID]string, fs *eventpb.SubmissionCompleted_CLSubmissionFailures) (string, error) {
	var sb strings.Builder
	for i, f := range fs.GetFailures() {
		if i > 0 {
			sb.WriteRune('\n')
		}
		if _, err := fmt.Fprintf(&sb, "CL %s failed to be submitted due to '%s'", clidToURL[common.CLID(f.GetClid())], f.GetMessage()); err != nil {
			return "", err
		}
	}
	return sb.String(), nil
}
