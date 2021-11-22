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

package handler

import (
	"context"

	"go.chromium.org/luci/auth/identity"

	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// requirementInput contains the information needed to compute the tryjob
// requirement.
type requirementInput struct {
	cg      *prjcfg.ConfigGroup
	owner   identity.Identity
	cls     []*run.RunCL
	options *run.Options
}

type invalidRequirementErr struct {
	// TODO(crbug/1257922): provide details
}

func (ir invalidRequirementErr) Error() string {
	// TODO(crbug/1257922): replace with concrete reason
	return "can't derive tryjob requirement"
}

func isInvalidRequirementErr(err error) bool {
	_, ok := err.(invalidRequirementErr)
	return ok
}

// computeRequirement computes the tryjob requirement to verify the run.
//
// Returns invalidRequirementErr if the provided input is invalid.
func computeRequirement(ctx context.Context, in requirementInput) (*tryjob.Requirement, error) {
	// TODO(crbug/1257922): implement
	return nil, nil
}
