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

package invoke

import (
	"bytes"
	"context"
	"os/exec"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/errors"

	bbpb "go.chromium.org/luci/buildbucket/proto"
)

// Subprocess represents a running luciexe.
type Subprocess struct {
	Step        *bbpb.Step
	parseOutput func() (*bbpb.Build, error)

	cmd *exec.Cmd
}

// Start invokes a binary implementing the luciexe protocol.
//
// Args:
//  * ctx will be used for deadlines/cancellation of the started luciexe.
//  * luciexePath must be the full absolute path to the luciexe binary.
//  * input must be the Build message you wish to pass to the luciexe binary.
//  * opts is optional (may be nil to take all defaults)
//
// This assumes that the current process is already operating within a "host
// application" environment. See "go.chromium.org/luci/luciexe" for details.
//
// The caller SHOULD take Subprocess.Step, append it to the current Build state,
// and send that (e.g. using `exe.WriteBuild`). Otherwise this luciexe's steps
// will not show up in the Build.
func Start(ctx context.Context, luciexePath string, input *bbpb.Build, opts *Options) (*Subprocess, error) {
	inputData, err := proto.Marshal(input)
	if err != nil {
		return nil, errors.Annotate(err, "marshalling input Build").Err()
	}

	launchOpts, err := opts.rationalize(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "normalizing options").Err()
	}

	cmd := exec.CommandContext(ctx, luciexePath, launchOpts.args...)
	cmd.Env = launchOpts.env.Sorted()
	cmd.Dir = launchOpts.workDir
	cmd.Stdin = bytes.NewBuffer(inputData)
	cmd.Stdout = launchOpts.stdout
	cmd.Stderr = launchOpts.stderr
	if err := cmd.Start(); err != nil {
		return nil, errors.Annotate(err, "launching luciexe").Err()
	}

	return &Subprocess{
		Step:        launchOpts.step,
		parseOutput: launchOpts.parseOutput,
		cmd:         cmd,
	}, nil
}

// Wait waits for the subprocess to terminate.
//
// If Options.CollectOutput was specified, this will return the final Build
// message, as reported by the luciexe.
//
// If you wish to cancel the subprocess (e.g. due to a timeout or deadline),
// make sure to pass a cancelable/deadline context to Start().
func (s *Subprocess) Wait() (*bbpb.Build, error) {
	if err := s.cmd.Wait(); err != nil {
		return nil, errors.Annotate(err, "waiting for luciexe").Err()
	}
	return s.parseOutput()
}
