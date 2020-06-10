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
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/luciexe/exe"
	"go.chromium.org/luci/luciexe/legacy/annotee"
	"go.chromium.org/luci/luciexe/legacy/annotee/annotation"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	exe.Run(func(ctx context.Context, build *pb.Build, sendBuild exe.BuildSender) error {
		opts := struct {
			Args []string `json:"args"`
		}{}
		err := exe.ParseProperties(build.Input.Properties, map[string]interface{}{
			"run_annotations": &opts,
		})
		check(err)

		args := opts.Args
		if len(args) == 0 {
			return errors.New("No arguments were provided")
		}

		cwd, err := os.Getwd()
		check(err)

		// Start the subprocess.
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		stdout, err := cmd.StdoutPipe()
		check(err)
		stderr, err := cmd.StderrPipe()
		check(err)
		err = cmd.Start()
		check(errors.Annotate(err, "failed to start subprocess").Err())

		ldBootstrap, err := bootstrap.Get()
		check(err)

		var buildMU sync.Mutex
		sendAnnotations := func(ann *milo.Step) {
			steps, err := annotee.ConvertBuildSteps(ctx, ann.Substep, false, "", "")
			check(errors.Annotate(err, "failed to extract steps from annotations").Err())

			props, err := milo.ExtractProperties(ann)
			check(errors.Annotate(err, "failed to extract properties from annotations").Err())

			buildMU.Lock()
			defer buildMU.Unlock()
			build.Steps = steps
			if build.Output == nil {
				build.Output = &pb.Build_Output{}
			}
			build.Output.Properties = props
			sendBuild()
		}

		// Run STDOUT/STDERR streams through the processor.
		// Run the process' output streams through Annotee. This will block until
		// they are all consumed.
		processor := annotee.New(ctx, annotee.Options{
			Client:                 ldBootstrap.Client,
			Execution:              annotation.ProbeExecution(os.Args, os.Environ(), cwd),
			MetadataUpdateInterval: 30 * time.Second,
			Offline:                false,
			CloseSteps:             true,
			AnnotationUpdated: func(annBytes []byte) {
				ann := &milo.Step{}
				check(errors.Annotate(proto.Unmarshal(annBytes, ann), "failed to parse annotation proto").Err())
				sendAnnotations(ann)
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
		check(errors.Annotate(err, "failed to process annotations").Err())

		// Wait for the subprocess to exit.
		switch err := cmd.Wait().(type) {
		case *exec.ExitError:
		case nil:
		default:
			check(errors.Annotate(err, "failed to wait for the subprocess to exit").Err())
		}

		// Send the final state.
		sendAnnotations(processor.Finish().RootStep().Proto())
		return nil
	})
}
