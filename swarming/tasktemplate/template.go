// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package tasktemplate resolves Swarming task templates.
//
// Templated strings may be passed to Swarming tasks in order to have them
// resolve task-side parameters. See Params for more information.
package tasktemplate

import (
	"github.com/luci/luci-go/common/data/text/stringtemplate"
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
