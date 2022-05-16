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

//go:build !linux
// +build !linux

package spantest

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"time"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/common/system/filesystem"
	"go.chromium.org/luci/common/system/port"
)

// StartEmulator starts a Cloud Spanner Emulator instance.
func StartEmulator(ctx context.Context) (*Emulator, error) {
	rand.Seed(time.Now().UTC().UnixNano())

	p, err := port.PickUnusedPort()
	if err != nil {
		return nil, errors.Annotate(err, "picking port").Err()
	}

	hostport := fmt.Sprintf("localhost:%d", p)
	e := &Emulator{
		hostport:      hostport,
		containerName: fmt.Sprintf("%d", rand.Int63()),
	}

	if e.cfgDir, err = e.createSpannerEmulatorConfig(); err != nil {
		return nil, err
	}

	ctx, e.cancel = context.WithCancel(ctx)
	e.cmd = exec.CommandContext(
		ctx, "docker", "run", "-p", fmt.Sprintf("127.0.0.1:%d:9010", p), "--name", e.containerName, "gcr.io/cloud-spanner-emulator/emulator")
	e.cmd.Env = append(e.cmd.Env,
		fmt.Sprintf("SPANNER_EMULATOR_HOST=%s", e.hostport),
		fmt.Sprintf("CLOUDSDK_CONFIG=%s", e.cfgDir),
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")))

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
		// if err := e.cmd.Process.Kill(); err != nil {
		// 	return err
		// }
		e.cancel()
		e.cmd = nil

		// Try to explicitly stop the container, because killing `docker run`
		// via context cancellation may not do it.
		stopCmd := exec.Command("docker", "stop", e.containerName)
		stopCmd.Start()
		return stopCmd.Wait()
	}

	if e.cfgDir != "" {
		if err := filesystem.RemoveAll(e.cfgDir); err == nil {
			return errors.Annotate(err, "failed to remove the temporary config directory").Err()
		}
		e.cfgDir = ""
	}
	return nil
}
