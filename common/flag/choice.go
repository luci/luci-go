// Copyright 2019 The LUCI Authors.
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

package flag

import (
	"strings"

	"go.chromium.org/luci/common/errors"
)

// Choice is an implementation of flag.Value for parsing a
// multiple-choice string.
type Choice struct {
	choices []string
	output  *string
}

// NewChoice creates a Choice value
func NewChoice(output *string, choices ...string) Choice {
	return Choice{choices: choices, output: output}
}

// String implements the flag.Value interface.
func (f Choice) String() string {
	if f.output == nil {
		return ""
	}
	return *f.output
}

// Set implements the flag.Value interface.
func (f Choice) Set(s string) error {
	if f.output == nil {
		return errors.New("Choice pointer is nil")
	}
	for _, choice := range f.choices {
		if s == choice {
			*f.output = s
			return nil
		}
	}
	valid := strings.Join(f.choices, ", ")
	return errors.Fmt("%s is not a valid choice; please select one of: %s", s, valid)
}
