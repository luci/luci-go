// Copyright 2024 The LUCI Authors.
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

package results

import (
	"fmt"
	"strings"
)

// value is a simple (name, value) tuple.
type value struct {
	name string

	// A verbatimString is rendered literally, anything else
	// is rendered with "%#v"
	value any
}

// render prints lines as a block of lines.
func (v *value) render() []string {
	if v == nil {
		return nil
	}
	val := ""
	if verbatim, ok := v.value.(verbatimString); ok {
		val = verbatim.String()
	} else {
		val = fmt.Sprintf("%#v", v.value)
	}
	if strings.TrimSpace(val) == "" {
		return []string{fmt.Sprintf("%s%s: <empty>", "  ", v.name)}
	}
	lines := strings.Split(val, "\n")
	return blockLines("  ", v.name, lines)
}

// if lines has zero lines, returns nil.
// if lines has one line, returns just that line prefixed with prefix and
// title.
// if lines has many lines, returns a prefixed title line followed by
// prefix+"| " prefixed lines.
//
// If lines has multiple blank lines in a row, they are collapsed into a single
// blank line.
//
// If lines ends with a blank line, it is dropped.
func blockLines(prefix, title string, lines []string) (ret []string) {
	if len(lines) == 0 {
		return nil
	}
	if len(lines) == 1 {
		if strings.TrimSpace(lines[0]) == "" {
			return
		}
		return []string{fmt.Sprintf("%s%s: %s", prefix, title, lines[0])}
	}
	buf := make([]string, 0, len(lines)+1)
	buf = append(buf, fmt.Sprintf("%s%s:", prefix, title))
	var needBlank bool
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			needBlank = true
			continue
		}
		if needBlank {
			buf = append(buf, fmt.Sprintf("%s|", prefix))
			needBlank = false
		}
		buf = append(buf, fmt.Sprintf("%s| %s", prefix, line))
	}
	if len(buf) > 0 {
		// could be all blank lines... this would be pretty weird, but may as well be
		// consistent.
		ret = buf
	}
	return
}
