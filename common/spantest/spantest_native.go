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

// +build linux

package spantest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/common/system/filesystem"
	"go.chromium.org/luci/common/system/port"
)

// emulatorRelativePath is the relative path of the Cloud Spanner Emulator binary to gcloud root.
const emulatorRelativePath = "bin/cloud_spanner_emulator/emulator_main"

// findEmulatorPath finds the path to Cloud Spanner Emulator binary.
//
// Note that this should only work on Linux because only gcloud on Linux contains
// Cloud Spanner Emulator component. For Windows and MacOS users, the emulator
// requires Docker to be installed on your system and available on the system path.
func findEmulatorPath() (string, error) {
	o, err := exec.Command("gcloud", "info", "--format=value(installation.sdk_root)").Output()
	if err != nil {
		return "", err
	}

	emulatorPath := filepath.Join(strings.TrimSuffix(string(o), "\n"), emulatorRelativePath)
	switch _, err = os.Stat(emulatorPath); {
	case os.IsNotExist(err):
		return "", fmt.Errorf("cannot find cloud spanner emulator binary at %v. \n Please run `make install-spanner-emulator`", emulatorPath)
	case err != nil:
		return "", err
	}

	return emulatorPath, nil
}

// StartEmulator starts a Cloud Spanner Emulator instance.
func StartEmulator(ctx context.Context) (*Emulator, error) {
	emulatorPath, err := findEmulatorPath()
	if err != nil {
		return nil, errors.Annotate(err, "find emulator").Err()
	}

	p, err := port.PickUnusedPort()
	if err != nil {
		return nil, errors.Annotate(err, "picking port").Err()
	}

	hostport := fmt.Sprintf("localhost:%d", p)
	e := &Emulator{
		hostport: hostport,
	}

	if e.cfgDir, err = e.createSpannerEmulatorConfig(); err != nil {
		return nil, err
	}

	ctx, e.cancel = context.WithCancel(ctx)
	e.cmd = exec.CommandContext(ctx, emulatorPath, "--host_port", e.hostport)
	e.cmd.Env = append(e.cmd.Env, fmt.Sprintf("SPANNER_EMULATOR_HOST=%s", e.hostport), fmt.Sprintf("CLOUDSDK_CONFIG=%s", e.cfgDir))
	e.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Without this, test will hang when finish.
	// But unfortunately this is only supported on Linux.
	e.cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	e.cmd.Stdout = os.Stdout
	e.cmd.Stderr = os.Stderr

	if err = e.cmd.Start(); err != nil {
		return nil, err
	}

	return e, nil
}

// Stop kills the emulator process and removes the temporary gcloud config directory.
func (e *Emulator) Stop() error {
	if e.cmd != nil {
		e.cmd.Process.Release()
		e.cancel()
		e.cmd = nil
	}

	if e.cfgDir != "" {
		if err := filesystem.RemoveAll(e.cfgDir); err == nil {
			return errors.Annotate(err, "failed to remove the temporary config directory").Err()
		}
		e.cfgDir = ""
	}
	return nil
}
