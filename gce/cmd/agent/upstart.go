// Copyright 2018 The LUCI Authors.
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

package main

import (
	"context"
	"os"
	"os/exec"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// Ensure UpstartStrategy implements PlatformStrategy.
var _ PlatformStrategy = &UpstartStrategy{}

// UpstartStrategy is a Linux-specific PlatformStrategy using upstart.
// Implements PlatformStrategy.
type UpstartStrategy struct {
	LinuxStrategy
}

// upstartCfg is the path to write the Swarming bot upstart config.
const upstartCfg = "/etc/init/swarming-start-bot.conf"

// upstartTmpl is the name of the Swarming bot upstart config template asset.
const upstartTmpl = "swarming-start-bot.conf.tmpl"

// upstartSrv is the name of the Swarming bot upstart service.
const upstartSrv = "swarming-start-bot"

// autostart configures the given Swarming bot code to be executed by the given
// python on startup for the given user, then starts the Swarming bot process.
// Implements PlatformStrategy.
func (*UpstartStrategy) autostart(c context.Context, path, user string, python string) error {
	subs := map[string]string{
		"BotCode": path,
		"User":    user,
		"Python":  python,
	}
	s, err := substitute(c, string(GetAsset(upstartTmpl)), subs)
	if err != nil {
		return errors.Fmt("failed to prepare template %q: %w", upstartTmpl, err)
	}

	logging.Infof(c, "installing: %s", upstartCfg)
	// 0644 allows the upstart config to be read by all users.
	// Useful when SSHing to the instance.
	if err := os.WriteFile(upstartCfg, []byte(s), 0644); err != nil {
		return errors.Fmt("failed to write: %s: %w", upstartCfg, err)
	}

	logging.Infof(c, "starting %q", upstartSrv)
	if err := exec.Command("initctl", "start", upstartSrv).Run(); err != nil {
		return errors.Fmt("failed to start service %q: %w", upstartSrv, err)
	}
	return nil
}

// canUseUpstart returns whether or not UpstartStrategy can be used.
func canUseUpstart() bool {
	return exec.Command("initctl", "--version").Run() == nil
}
