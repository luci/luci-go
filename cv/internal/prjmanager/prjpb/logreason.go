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

package prjpb

import (
	"sort"
	"strings"
)

// SortAndDedupeLogReasons does what its name says without modifying input.
func SortAndDedupeLogReasons(in []LogReason) []LogReason {
	var out []LogReason
	if len(in) == 0 {
		return out
	}
	out = append(out, in...) // copy
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	prevUniqIndex := 0
	for _, o := range out {
		if out[prevUniqIndex] == o {
			continue
		}
		prevUniqIndex++
		out[prevUniqIndex] = o
	}
	return out[:prevUniqIndex+1]
}

// FormatLogReasons produces "[HUMAN, READABLE]" string for a list of statuses.
func FormatLogReasons(in []LogReason) string {
	sb := strings.Builder{}
	sb.WriteRune('[')
	for i, r := range in {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(r.String())
	}
	sb.WriteRune(']')
	return sb.String()
}
