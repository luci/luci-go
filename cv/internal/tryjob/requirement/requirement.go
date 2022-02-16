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

package requirement

import (
	"context"
	"fmt"

	"go.chromium.org/luci/auth/identity"

	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// Input contains all info needed to compute the Tryjob Requirement.
type Input struct {
	ConfigGroup *prjcfg.ConfigGroup
	RunOwner    identity.Identity
	CLs         []*run.RunCL
	RunOptions  *run.Options
}

// ComputationFailure is what fails the Tryjob Requirement computation.
type ComputationFailure interface {
	// Reason returns a human-readable string that explains what fails the
	// requirement computation.
	//
	// This will be shown directly to users. Make sure it doesn't leak any
	// information.
	Reason() string
}

// ComputationResult is the result of Tryjob Requirement computation.
type ComputationResult struct {
	// Requirement is the derived Tryjob Requirement to verify the Run.
	//
	// Mutually exclusive with `ComputationFailure`.
	Requirement *tryjob.Requirement
	// ComputationFailure is what fails the requirement computation.
	//
	// This is different from the returned error from the `Compute` function.
	// This failure is typically caused by invalid directive from users or
	// Project Config (e.g. including a builder that is not defined in Project
	// config via git-footer). It should be reported back to the user to decide
	// the next step. On the other hand, the returned error from the `Compute`
	// function is typically caused by internal errors, like remote RPC call
	// failure, and should generally be retried.
	//
	// Mutually exclusive with `Requirement`.
	ComputationFailure ComputationFailure
}

// OK returns true if successfully computed Tryjob requirement.
//
// `ComputationFailure` MUST be present if returns false.
func (r ComputationResult) OK() bool {
	switch {
	case r.ComputationFailure != nil:
		if r.Requirement != nil {
			panic(fmt.Errorf("both Requirement and ComputationFailure are present"))
		}
		return false
	case r.Requirement == nil:
		panic(fmt.Errorf("neither Requirement nor ComputationFailure is present"))
	default:
		return true
	}
}

// Compute computes the Tryjob requirement to verify the run.
func Compute(ctx context.Context, in Input) (ComputationResult, error) {
	// TODO(crbug/1257922): implement
	return ComputationResult{Requirement: &tryjob.Requirement{}}, nil
}
