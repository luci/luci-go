// Copyright 2017 The LUCI Authors.
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

// Package tasktemplate resolves Swarming task templates.
//
// Templated strings may be passed to Swarming tasks in order to have them
// resolve task-side parameters. See Params for more information.
package tasktemplate

import (
	"go.chromium.org/luci/common/data/text/stringtemplate"
)

// Params contains supported Swarming task string substitution parameters.
//
// A template string can be resolved by passing it to a Params instance's
// Resolve method.
type Params struct {
	// SwarmingRunID is the substitution to use for the "swarming_run_id" template
	// parameter.
	//
	// Note that this is the Swarming Run ID, not Task ID. The Run ID is the
	// combination of the Task ID with the try number.
	SwarmingRunID string
}

func (p *Params) substMap() map[string]string {
	res := make(map[string]string, 1)
	if p.SwarmingRunID != "" {
		res["swarming_run_id"] = p.SwarmingRunID
	}
	return res
}

// Resolve resolves v against p's parameters, populating any template fields
// with their respective parameter value.
//
// If the string is invalid, or if it references an undefined template
// parameter, an error will be returned.
func (p *Params) Resolve(v string) (string, error) {
	return stringtemplate.Resolve(v, p.substMap())
}
