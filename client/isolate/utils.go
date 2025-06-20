// Copyright 2015 The LUCI Authors.
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

package isolate

import (
	"log"

	"go.chromium.org/luci/common/errors"
)

func must(condition bool, info ...any) {
	if condition {
		return
	}
	if len(info) == 0 {
		log.Panic(errors.RenderStack(errors.New("assertion failed")))
	} else if format, ok := info[0].(string); ok {
		log.Panic(errors.RenderStack(errors.Fmt("assertion failed: "+format, info[1:])))
	}
}

// uniqueMergeSortedStrings merges two sorted sets of string (as slices) and removes duplicates.
func uniqueMergeSortedStrings(ls, rs []string) []string {
	varSet := make([]string, len(ls)+len(rs))
	for i := 0; ; i++ {
		if len(ls) == 0 {
			rs, ls = ls, rs
		}
		if len(rs) == 0 {
			i += copy(varSet[i:], ls)
			return varSet[:i]
		}
		must(i < len(varSet))
		must(len(rs) > 0 && len(ls) > 0)
		if ls[0] > rs[0] {
			ls, rs = rs, ls
		}
		if ls[0] < rs[0] {
			varSet[i] = ls[0]
			ls = ls[1:]
		} else {
			varSet[i] = ls[0]
			ls, rs = ls[1:], rs[1:]
		}
	}
}
