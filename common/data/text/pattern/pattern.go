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

// Package pattern implements lightweight parsable string patterns.
package pattern

import (
	"fmt"
	"regexp"
	"strings"
)

// Pattern can either match or not match a string.
type Pattern interface {
	// String returns the definition of the pattern parsable by Parse.
	String() string
	// Match returns true if s matches this pattern, otherwise false.
	Match(s string) bool
}

// Parse parses a pattern.
//
// Ordered by precedence, s can be:
//
//  - "": matches nothing
//  - "*": matches anything
//  - "<S>" where S does not have a colon: same as "exact:<S>"
//  - "exact:<S>": matches only string S
//  - "text:<S>": same as "exact:<S>" for backward compatibility
//  - "regex:<E>": matches all strings matching regular expression E. If E
//    does not start/end with ^/$, they are added automatically.
//
// Anything else will cause an error.
func Parse(s string) (Pattern, error) {
	switch s {
	case "":
		return None, nil
	case "*":
		return Any, nil
	}

	parts := strings.SplitN(s, ":", 2)
	if len(parts) < 2 {
		return Exact(s), nil
	}

	kind, value := parts[0], parts[1]
	switch kind {
	case "exact", "text":
		return Exact(value), nil
	case "regex":
		switch value {
		case ".", ".*":
			return Any, nil
		case "^$":
			return None, nil
		default:
			if !strings.HasPrefix(value, "^") {
				value = "^" + value
			}
			if !strings.HasSuffix(value, "$") {
				value = value + "$"
			}
			r, err := regexp.Compile(value)
			if err != nil {
				return nil, err
			}
			return Regexp(r), nil
		}
	default:
		return nil, fmt.Errorf("unknown pattern kind: %q", kind)
	}
}

// MustParse parses the pattern according to the specification
// of Parse. In addition, it panics if there is an error in parsing the
// given string as a pattern.
//
// See Parse for more details.
func MustParse(s string) Pattern {
	pattern, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return pattern
}
