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

// +build linux

// Contains platform-specific implementation for Linux.
package main

import (
	"os"
	"os/exec"
	"os/user"
	"strconv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"golang.org/x/net/context"
)

type LinuxStrategy struct {
}

// Change ownership of a path.
func (strategy LinuxStrategy) chown(ctx context.Context, username string, path string) error {
	user, err := user.Lookup(username)
	if (err != nil) {
		return err
	}
	uid, err := strconv.Atoi(user.Uid)
	if (err != nil) {
		return err
	}
	gid, err := strconv.Atoi(user.Gid)
	if (err != nil) {
		return err
	}
	return os.Chown(path, uid, gid)
}

// Reboots the machine.
func (strategy LinuxStrategy) reboot(ctx context.Context) error {
	cmd := exec.Command("/sbin/shutdown", "-r", "now")
	err := cmd.Start()
	if (err != nil) {
		return err
	}
	return cmd.Wait()
}

type SystemdStrategy struct{
	LinuxStrategy
}

// Starts the agent.
func (strategy SystemdStrategy) start(ctx context.Context, _ string) error {
	cmd := exec.Command("systemctl", "daemon-reload")
	err := cmd.Start()
	if (err != nil) {
		return err
	}
	cmd = exec.Command("systemctl", "enable", "machine-provider-agent")
	err = cmd.Start()
	if (err != nil) {
		return err
	}
	cmd = exec.Command("systemctl", "start", "machine-provider-agent")
	err = cmd.Start()
	if (err != nil) {
		return err
	}
	return nil
}

// Stops the agent.
func (strategy SystemdStrategy) stop(ctx context.Context) error {
	cmd := exec.Command("systemctl", "stop", "machine-provider-agent")
	err := cmd.Start()
	if (err != nil) {
		return err
	}
	return nil
}

type UpstartStrategy struct{
	LinuxStrategy
}

// Starts the agent.
func (strategy UpstartStrategy) start(ctx context.Context, _ string) error {
	cmd := exec.Command("initctl", "reload-configuration")
	err := cmd.Start()
	if (err != nil) {
		return err
	}
	cmd = exec.Command("start", "machine-provider-agent")
	err = cmd.Start()
	if (err != nil) {
		return err
	}
	return nil
}

// Stops the agent.
func (strategy UpstartStrategy) stop(ctx context.Context) error {
	cmd := exec.Command("stop", "machine-provider-agent")
	err := cmd.Start()
	if (err != nil) {
		return err
	}
	return nil
}

// Returns true iff systemd is supported on this machine.
func systemdFound() bool {
	cmd := exec.Command("which", "systemctl")
	err := cmd.Start()
	if (err != nil) {
		return false
	}
	err = cmd.Wait()
	return (err == nil)
}

// Returns true iff upstart is supported on this machine.
func upstartFound() bool {
	cmd := exec.Command("init", "--version")
	err := cmd.Start()
	if (err != nil) {
		return false
	}
	err = cmd.Wait()
	return (err == nil)
}

// Returns an agent which runs on Linux, depending on supported init systems.
func getAgent(ctx context.Context) (*Agent, error) {
	if (systemdFound()) {
		agent := Agent {
			agentAutoStartPath: "/etc/systemd/system/machine-provider-agent.service",
			agentAutoStartTemplate: "machine-provider-agent.service.tmpl",
			logsDir: "/var/log/machine-provider-agent",
			swarmingAutoStartPath: "/etc/systemd/system/swarming-start-bot.service",
			swarmingAutoStartTemplate: "swarming-start-bot.service.tmpl",
			swarmingBotDir: "/b/s",
			strategy: SystemdStrategy{LinuxStrategy{}},
		}
		logging.Infof(ctx, "Using systemd Linux agent.")
		return &agent, nil
	}

	if (upstartFound()) {
		agent := Agent {
			agentAutoStartPath: "/etc/init/machine-provider-agent.conf",
			agentAutoStartTemplate: "machine-provider-agent.conf.tmpl",
			logsDir: "/var/log/messages/machine-provider-agent",
			swarmingAutoStartPath: "/etc/init/swarming-start-bot.conf",
			swarmingAutoStartTemplate: "swarming-start-bot.conf.tmpl",
			swarmingBotDir: "/b/s",
			strategy: UpstartStrategy{LinuxStrategy{}},
		}
		logging.Infof(ctx, "Using Upstart Linux agent.")
		return &agent, nil
	}

	return nil, errors.New("Unsupported init system, expected systemd or Upstart.")
}
