// Copyright 2022 The LUCI Authors.
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

// Package cv exposes CV properties to luciexe binaries for builds.
package cv

import (
	"context"
	"errors"

	cv "go.chromium.org/luci/cv/api/recipe/v1"

	"go.chromium.org/luci/luciexe/build"
)

// TODO(yiwzhang): This should probably have it's own namespace rather than
// taking $recipe_engine/cq, but we'd need to plumb in that information.
var inProps = build.RegisterInputProperty[*cv.Input]("$recipe_engine/cq")

var ErrNotActive = errors.New("LUCI CV is not active for this build")

// RunMode returns the name of the CQ run mode.
//
// If CV is not active for this build, returns ErrNotActive.
func RunMode(ctx context.Context) (string, error) {
	i := inProps.GetInput(ctx)
	if i == nil || !i.Active {
		return "", ErrNotActive
	}
	return i.RunMode, nil
}
