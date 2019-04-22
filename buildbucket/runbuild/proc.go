// Copyright 2019 The LUCI Authors.
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

package runbuild

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"

	"github.com/golang/protobuf/proto"
	pb "go.chromium.org/luci/buildbucket/proto"
)

// userSubprocess is a subprocess of a user executable.
type userSubprocess struct {
	Path      string
	Dir       string
	Env       environ.Env
	InitBuild *pb.Build

	cmd *exec.Cmd

	stdout io.ReadCloser
	stderr io.ReadCloser
}

func (p *userSubprocess) Start(ctx context.Context) (stdout, stderr io.ReadCloser, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		if err != nil {
			cancel()
		}
	}()

	cmd := exec.CommandContext(ctx, p.Path)
	cmd.Dir = p.Dir
	cmd.Env = p.Env.Sorted()

	buildBytes, err := proto.Marshal(p.InitBuild)
	if err != nil {
		return
	}
	cmd.Stdin = bytes.NewReader(buildBytes)

	stdout, err = cmd.StdoutPipe()
	if err != nil {
		return
	}

	stderr, err = cmd.StderrPipe()
	if err != nil {
		return
	}

	err = cmd.Start()
	if err != nil {
		return
	}

	p.cmd = cmd
	return
}

// Wait returns an error only if waiting fails.
// If the subrocess exits with a non-zero code, logs the exit code and returns
// nil.
// Context is used for logging.
func (p *userSubprocess) Wait(ctx context.Context) error {
	if p.cmd == nil {
		return fmt.Errorf("not started")
	}

	switch err := p.cmd.Wait().(type) {
	case *exec.ExitError:
		// The subprocess has started successfully, but exited with a non-zero
		// exit code. This is OK. Exit code is not a part of user executable
		// contract.
		logging.Infof(ctx, "user subprocess exited with code %d", err.ExitCode())
		return nil

	case nil:
		logging.Infof(ctx, "user subprocess has exited with zero code")
		return nil

	default:
		return errors.Annotate(err, "failed to wait for the user executable to exit").Err()
	}
}
