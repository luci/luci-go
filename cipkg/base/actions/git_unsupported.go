// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build aix && ppc64

// Disable git on unsupported platforms:
// - github.com/go-git/go-billy/v5/osfs doesn't support aix-ppc64.

package actions

import (
	"context"
	"fmt"

	"go.chromium.org/luci/cipkg/core"
)

// ActionGitFetchTransformer is the stub for core.ActionGitFetch on unsupported platforms.
func ActionGitFetchTransformer(a *core.ActionGitFetch, deps []Package) (*core.Derivation, error) {
	return nil, fmt.Errorf("ActionGitFetch is not supported on the platform")
}

// ActionGitFetchExecutor is the stub for core.ActionGitFetch on unsupported platforms.
func ActionGitFetchExecutor(ctx context.Context, a *core.ActionGitFetch, out string) error {
	return fmt.Errorf("ActionGitFetch is not supported on the platform")
}
