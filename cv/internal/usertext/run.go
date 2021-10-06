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

// OnRunSucceeded returns notification messages for a Run success.
//
// notify is the message posted on the CL to inform the success.
// This can be lengthy with details.
//
// reason is the message set in the attention set, so that the users can find
// the reason of the attention set in UI. It's better to keep this message
// as concise as possible.
func OnRunSucceeded(mode run.Mode) (notify, reason string) {
	switch mode {
	case run.QuickDryRun:
		notify = "Quick dry run: This CL passed the CQ quick dry run."
		reason = "CQ quick dry run succeeded."
	case run.DryRun:
		notify = "Dry run: This CL passed the CQ dry run."
		reason = "CQ dry run succeeded."
	default:
		panic(fmt.Sprintf("impossible Run mode %q", mode))
	}
	return
}

// OnFullRunSucceeded returns a notification message for a successful full run.
func OnFullRunSucceeded(mode run.Mode) string {
	switch mode {
	case run.FullRun:
		return "Full run: This CL passed CQ and is ready for submission."
	default:
		panic(fmt.Sprintf("impossible Run mode %q", mode))
	}
}

// OnRunFailed returns notification messages for a Run failure.
//
// notify is the message posted on the CL to inform the failure.
// This can be lengthy with details.
//
// reason is the message set in the attention set, so that the users can find
// the reason of the attention set in UI. It's better to keep this message
// as concise as possible.
func OnRunFailed(mode run.Mode) (notify, reason string) {
	switch mode {
	case run.QuickDryRun:
		notify = "Quick dry run: This CL failed the CQ quick dry run."
		reason = "CQ quick dry run failed."
	case run.DryRun:
		notify = "Dry run: This CL failed the CQ dry run."
		reason = "CQ dry run failed."
	case run.FullRun:
		notify = "Full run: This CL failed the CQ full run."
		reason = "CQ full run failed."
	default:
		panic(fmt.Sprintf("impossible Run mode %q", mode))
	}
	return
}
