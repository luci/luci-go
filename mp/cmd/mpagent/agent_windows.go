// Copyright 2017 The LUCI Authors.
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
	"syscall"

	"go.chromium.org/luci/common/logging"
)

type WindowsStrategy struct {
}

// chown changes ownership of a path.
func (WindowsStrategy) chown(ctx context.Context, username, path string) error {
	// TODO(smut): Determine if this is necessary on Windows.
	return nil
}

// enableSwarming enables installed service.
func (WindowsStrategy) enableSwarming(ctx context.Context) error {
	return nil
}

// reboot reboots the machine.
func (WindowsStrategy) reboot(ctx context.Context) error {
	return exec.Command("shutdown", "/f", "/r", "/t", "0").Run()
}

// start starts the agent.
func (WindowsStrategy) start(ctx context.Context, path string) error {
	cmd := exec.Command(path)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// https://msdn.microsoft.com/en-us/library/windows/desktop/ms684863.aspx
		// CREATE_NEW_PROCESS_GROUP: 	0x200
		// DETACHED_PROCESS: 		0x008
		CreationFlags: 0x200 | 0x8,
	}
	return cmd.Start()
}

// stop stops all instances of the agent.
func (WindowsStrategy) stop(ctx context.Context) error {
	// TODO(smut): Stop the agent.
	return nil
}

// getAgent returns an agent which runs on Windows.
func getAgent(ctx context.Context) (*Agent, error) {
	agent := Agent{
		agentAutoStartPath:        "C:\\Users\\{{.User}}\\Start Menu\\Programs\\Startup\\machine-provider-agent.bat",
		agentAutoStartTemplate:    "machine-provider-agent.bat.tmpl",
		logsDir:                   "C:\\logs",
		swarmingAutoStartPath:     "C:\\Users\\{{.User}}\\Start Menu\\Programs\\Startup\\swarming-start-bot.bat",
		swarmingAutoStartTemplate: "swarming-start-bot.bat.tmpl",
		swarmingBotDir:            "C:\\b\\s",
		strategy:                  WindowsStrategy{},
	}
	logging.Infof(ctx, "Using Windows agent.")
	return &agent, nil
}
