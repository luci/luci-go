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

package execute

import (
	"fmt"
	"strings"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/tryjob"
)

type intSet map[int]struct{}

func (is intSet) has(value int) bool {
	_, ret := is[value]
	return ret
}

// composeReason puts together information about failing tryjobs.
func composeReason(tryjobs []*tryjob.Tryjob) string {
	if len(tryjobs) == 0 {
		panic(fmt.Errorf("composeReason called without tryjobs"))
	}
	var sb strings.Builder
	sb.WriteString("Failed Tryjobs:")
	for _, tj := range tryjobs {
		sb.WriteString("\n* ")
		sb.WriteString(tj.ExternalID.MustURL())
		if sm := tj.Result.GetBuildbucket().GetSummaryMarkdown(); sm != "" && tj.Definition.ResultVisibility != cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED {
			for _, line := range strings.Split(sm, "\n") {
				sb.WriteString("\n ") // indent.
				sb.WriteString(line)
			}
		}
	}
	return sb.String()
}
