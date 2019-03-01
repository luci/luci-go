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
	"os/exec"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// Ensure WindowsStrategy implements PlatformStrategy.
var _ PlatformStrategy = &WindowsStrategy{}

// WindowsStrategy is a Windows-specific PlatformStrategy.
// Implements PlatformStrategy.
type WindowsStrategy struct {
}

// startupTmpl is the Swarming bot startup task command template.
const startupTmpl = "C:\\tools\\python\\bin\\python.exe {{.BotCode}} start_bot"

// startupTask is the name of the Swarming bot startup task.
const startupTask = "swarming-start-bot"

// autostart configures the given Swarming bot code to be executed on startup
// for the given user, then starts the Swarming bot process.
// Implements PlatformStrategy.
func (*WindowsStrategy) autostart(c context.Context, path, user string) error {
	subs := map[string]string{
		"BotCode": path,
		"User":    user,
	}
	s, err := substitute(c, startupTmpl, subs)
	if err != nil {
		return errors.Annotate(err, "failed to prepare template: %s", startupTmpl).Err()
	}

	logging.Infof(c, "installing %q", startupTask)
	// Prevent the Swarming bot process from configuring its own autostart.
	if err := exec.Command("setx", "/m", "SWARMING_EXTERNAL_BOT_SETUP", "1").Run(); err != nil {
		return errors.Annotate(err, "failed to setx environment variable").Err()
	}
	if err := exec.Command("schtasks", "/create", "/f", "/it", "/rl", "HIGHEST", "/ru", user, "/sc", "onlogon", "/tn", startupTask, "/tr", s).Run(); err != nil {
		return errors.Annotate(err, "failed to create task %q", startupTask).Err()
	}

	logging.Infof(c, "starting %q", startupTask)
	if err := exec.Command("schtasks", "/run", "/tn", startupTask).Run(); err != nil {
		return errors.Annotate(err, "failed to start task %q", startupTask).Err()
	}
	return nil
}

// chown modifies the given path to be owned by the given user.
// Implements PlatformStrategy.
func (*WindowsStrategy) chown(c context.Context, path, username string) error {
	if err := exec.Command("icacls", path, "/setowner", username).Run(); err != nil {
		return errors.Annotate(err, "failed to set owner: %s", path).Err()
	}
	return nil
}
