// Copyright 2024 The LUCI Authors.
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

package emulator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync/atomic"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
)

// counts number of container started by the process.
var counter atomic.Int64

// startAsContainer attempts to launch Cloud Spanner emulator as a container.
func startAsContainer(ctx context.Context, driver string, out io.Writer) (grpcAddr string, stop func(), err error) {
	rnd := make([]byte, 4)
	if _, err := rand.Read(rnd); err != nil {
		panic(err)
	}
	container := fmt.Sprintf("cloud-spanner-%d-%d-%s",
		os.Getpid(),
		counter.Add(1),
		hex.EncodeToString(rnd),
	)

	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, driver, "run",
		"-p", "127.0.0.1::9010", // 9010 in the container => an available localhost port
		"--name", container,
		"gcr.io/cloud-spanner-emulator/emulator:latest",
	)
	cmd.Stdout = out
	cmd.Stderr = out

	if err = cmd.Start(); err != nil {
		cancel()
		return "", nil, err
	}

	// Terminates the container, best effort.
	stop = func() {
		cancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = exec.CommandContext(stopCtx, driver, "stop", container).Run()
		_ = cmd.Wait()
	}

	// Kill if exiting with an error or a panic.
	done := false
	defer func() {
		if err != nil || !done {
			stop()
		}
	}()

	// Inspect this container to get the assigned port number. It may not be up
	// yet, since we run the command asynchronously. So retry a bunch of times.
	var info *containerInfo
	inspectCtx, inspectCancel := context.WithTimeout(ctx, 30*time.Second)
	defer inspectCancel()
	attempt := 0
	for inspectCtx.Err() == nil {
		attempt++
		info, err = inspectContainer(inspectCtx, driver, container)
		if err == nil {
			break
		}
		clock.Sleep(inspectCtx, 10*time.Millisecond*time.Duration(min(attempt, 100)))
	}
	if inspectCtx.Err() != nil {
		err = errors.Fmt("timeout trying to query container info, the last error: %s", err)
		return
	}

	// Fish out the host port assigned to the gRPC emulator port.
	grpcPort := info.NetworkSettings.Ports["9010/tcp"]
	if len(grpcPort) == 0 {
		err = errors.Fmt("no 9010/tcp port in the container network settings %v", info)
		return
	}
	grpcAddr = fmt.Sprintf("127.0.0.1:%s", grpcPort[0].HostPort)

	done = true
	return
}

// Subset of the JSON dump `$driver inspect ...` returns.
type containerInfo struct {
	NetworkSettings struct {
		Ports map[string][]struct {
			HostPort string
		}
	}
}

// inspectContainer calls `$driver inspect ...`.
func inspectContainer(ctx context.Context, driver, container string) (*containerInfo, error) {
	out, err := exec.CommandContext(ctx, driver, "inspect", container).Output()
	if err != nil {
		return nil, err
	}
	var info []containerInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, err
	}
	if len(info) != 1 {
		return nil, errors.Fmt("unexpected container info returned: %v", info)
	}
	return &info[0], nil
}
