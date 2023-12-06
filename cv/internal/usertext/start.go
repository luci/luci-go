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

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/run"
)

const body = "CV is trying the patch."

// OnRunStarted generates a starting message for humans.
func OnRunStarted(mode run.Mode) string {
	if mode == run.DryRun {
		return "Dry run: " + body
	}
	return body
}

// OnRunStartedGerritMessage generates a starting message to be posted on each
// of the Gerrit CLs involved in the Run.
func OnRunStartedGerritMessage(r *run.Run, cfg *prjcfg.ConfigGroup, env *common.Env) string {
	msg := OnRunStarted(r.Mode)
	// TODO(crbug/1233963): always post a URL after ACLs are everywhere.
	if cfg.CQStatusHost != "" {
		msg += fmt.Sprintf("\n\nFollow status at: %s/ui/run/%s", env.HTTPAddressBase, r.ID)
	}
	return msg
}
