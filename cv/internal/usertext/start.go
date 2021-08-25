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

package usertext

import (
	"fmt"

	"go.chromium.org/luci/cv/internal/run"
)

func OnRunStarted(mode run.Mode) string {
	// TODO(tandrii): change to CV once CV posting the message sticks in
	// production, because this may affect user's email filters.
	const suffix = "CQ is trying the patch."
	switch mode {
	case run.QuickDryRun:
		return "Quick dry run: " + suffix
	case run.DryRun:
		return "Dry run: " + suffix
	case run.FullRun:
		return suffix
	default:
		panic(fmt.Sprintf("impossible Run mode %q", mode))
	}
}
