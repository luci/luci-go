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

// Command run_annotations is a LUCI executable that wraps a command that
// produces @@@ANNOTATIONS@@@, and converts annotations to build message.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/buildbucket/deprecated"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/logdog/client/annotee"
	"go.chromium.org/luci/logdog/client/annotee/annotation"
	"go.chromium.org/luci/luciexe/exe"
)

var (
	build   *pb.Build
	buildMU sync.Mutex
)

// check marks the build as INFRA_FAILURE and exits with code 1 if err is not nil.
func check(ctx context.Context, err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)

	buildMU.Lock()
	defer buildMU.Unlock()
	build.Status = pb.Status_INFRA_FAILURE
	build.SummaryMarkdown = fmt.Sprintf("run_annotations failure: `%s`", err)
	if err = exe.WriteBuild(ctx, build); err != nil {
		fmt.Fprintln(os.Stderr, "While writing final build: ", err)
	}
	os.Exit(1)
}

// sendAnnotations parses steps and properties from ann, updates build and sends
// to the caller.
func sendAnnotations(ctx context.Context, ann *milo.Step) error {
	logdog := exe.GetLogdog()
	fullPrefix := fmt.Sprintf("%s/%s", logdog.Prefix, logdog.Namespace)
	steps, err := deprecated.ConvertBuildSteps(ctx, ann.Substep, logdog.CoordinatorHost, fullPrefix)
	if err != nil {
		return errors.Annotate(err, "failed to extra steps from annotations").Err()
	}

	props, err := milo.ExtractProperties(ann)
	if err != nil {
		return errors.Annotate(err, "failed to extract properties from annotations").Err()
	}

	buildMU.Lock()
	defer buildMU.Unlock()
	build.Steps = steps
	build.Output.Properties = props
	return errors.Annotate(exe.WriteBuild(ctx, build), "failed to write build message").Err()
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	signals.HandleInterrupt(cancel)

	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: run_annotations ./my_program.sh args for my program")
		os.Exit(1)
	}

	if err := exe.Initialize(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "failed to initialize LUCI Executable client")
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	check(ctx, err)

	// Start the subprocess.
	cmd := exec.CommandContext(ctx, args[1], args[2:]...)
	stdout, err := cmd.StdoutPipe()
	check(ctx, err)
	stderr, err := cmd.StderrPipe()
	check(ctx, err)
	err = cmd.Start()
	check(ctx, errors.Annotate(err, "failed to start subprocess").Err())

	// Run STDOUT/STDERR streams through the processor.
	// Run the process' output streams through Annotee. This will block until
	// they are all consumed.
	processor := annotee.New(ctx, annotee.Options{
		Client:                 exe.GetLogdog().Client,
		Execution:              annotation.ProbeExecution(os.Args, os.Environ(), cwd),
		MetadataUpdateInterval: 30 * time.Second,
		Offline:                false,
		CloseSteps:             true,
		AnnotationUpdated: func(annBytes []byte) {
			ann := &milo.Step{}
			check(ctx, errors.Annotate(proto.Unmarshal(annBytes, ann), "failed to parse annotation proto").Err())
			check(ctx, sendAnnotations(ctx, ann))
		},
	})
	streams := []*annotee.Stream{
		{
			Reader:   stdout,
			Name:     annotee.STDOUT,
			Annotate: true,
		},
		{
			Reader:   stderr,
			Name:     annotee.STDERR,
			Annotate: true,
		},
	}
	err = processor.RunStreams(streams)
	check(ctx, errors.Annotate(err, "failed to process annotations").Err())

	// Wait for the subprocess to exist.
	switch err := cmd.Wait().(type) {
	case *exec.ExitError:
	case nil:
	default:
		check(ctx, errors.Annotate(err, "failed to wait for the subprocess to exit").Err())
	}

	// Send the final state.
	check(ctx, sendAnnotations(ctx, processor.Finish().RootStep().Proto()))
}
