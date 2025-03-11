// Copyright 2025 The LUCI Authors.
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

// Package internal contains some internal lucicfg helpers.
package internal

import (
	"context"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// testingTweaksKey is used as a context key.
var testingTweaksKey = "lucicfg.internal.TestingTweaks"

// TestingTweaks live in the context and affect behavior of some functions when
// they are used by lucicfg own tests.
type TestingTweaks struct {
	SkipEntrypointCheck     bool // do not check pkg.entrypoint(...)
	SkipPackageCompatChecks bool // do not check lucicfg.check_version(...) matches pkg.declare(...) and similar
}

// ToStruct returns a matching Starlark struct.
func (tt *TestingTweaks) ToStruct() *starlarkstruct.Struct {
	return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"skip_entrypoint_check":      starlark.Bool(tt.SkipEntrypointCheck),
		"skip_package_compat_checks": starlark.Bool(tt.SkipPackageCompatChecks),
	})
}

var noTweaks = TestingTweaks{}

// WithTestingTweaks puts testing tweaks into the context.
func WithTestingTweaks(ctx context.Context, tw *TestingTweaks) context.Context {
	return context.WithValue(ctx, &testingTweaksKey, tw)
}

// GetTestingTweaks returns the testing tweaks in the context or defaults.
func GetTestingTweaks(ctx context.Context) *TestingTweaks {
	if val, _ := ctx.Value(&testingTweaksKey).(*TestingTweaks); val != nil {
		return val
	}
	return &noTweaks
}
