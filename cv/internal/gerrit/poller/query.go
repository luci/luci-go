// Copyright 2020 The LUCI Authors.
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

package poller

import (
	"strings"
)

func buildQuery(sp *SubPoller) string {
	buf := strings.Builder{}
	buf.WriteString("status:NEW ")
	// TODO(tandrii): make label optional to support Tricium use-case.
	buf.WriteString("label:Commit-Queue>0 ")

	emitProjectValue := func(p string) {
		// Even though it appears to work without, Gerrit doc says project names
		// containing / must be surrounded by "" or {}:
		// https://gerrit-review.googlesource.com/Documentation/user-search.html#_argument_quoting
		buf.WriteRune('"')
		buf.WriteString(p)
		buf.WriteRune('"')
	}

	// One of .OrProjects or .CommonProjectPrefix must be set.
	switch prs := sp.GetOrProjects(); len(prs) {
	case 0:
		if sp.GetCommonProjectPrefix() == "" {
			panic("partitionConfig function should have ensured this")
		}
		// project*s* means find matching projects by prefix
		buf.WriteString("projects:")
		emitProjectValue(sp.GetCommonProjectPrefix())
	case 1:
		buf.WriteString("project:")
		emitProjectValue(prs[0])
	default:
		buf.WriteRune('(')
		for i, p := range prs {
			if i > 0 {
				buf.WriteString(" OR ")
			}
			buf.WriteString("project:")
			emitProjectValue(p)
		}
		buf.WriteRune(')')
	}
	return buf.String()
}
