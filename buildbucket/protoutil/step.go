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

package protoutil

import (
	"strings"

	"go.chromium.org/luci/common/errors"
)

// StepNameSep is the separator between each node in step names.
const StepNameSep = "|"

// ParentStepName returns the name of the parent step.
//
// If stepName is empty or doesn't have a parent, an empty string is returned.
func ParentStepName(stepName string) string {
	i := strings.LastIndex(stepName, StepNameSep)
	if i == -1 {
		return ""
	}
	return stepName[:i]
}

// ValidateStepName validates a given step name.
func ValidateStepName(stepName string) error {
	if stepName == "" {
		return errors.New("required")
	}

	nodes := strings.Split(stepName, StepNameSep)
	for _, node := range nodes {
		if node == "" {
			return errors.Fmt("there must be at least one character before and after %q (%q)",
				StepNameSep, stepName)
		}
	}
	return nil
}
