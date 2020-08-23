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

package exe

import (
	"bytes"
	"compress/zlib"
	"context"
	"flag"
	"io"
	"io/ioutil"
	"os"
	"runtime/debug"
	"time"

	"github.com/golang/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"golang.org/x/time/rate"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/luciexe"
)

// MainFn is the function signature you must implement in your callback to Run.
//
// Args:
//  - ctx: The context will be canceled when the program receives the os
//   Interrupt or SIGTERM (on unix) signal. The context has the following
//   libraries enabled:
//      * go.chromium.org/luci/common/logging
//      * go.chromium.org/luci/common/system/environ
//      * The Build modification functions in this package (i.e. everything
//        enabled by SinkBuildUpdates).
//  - build: The Build state, initialized with the Build read from stdin.
//  - userArgs: All command line arguments supplied after first `--`.
//
// Note: You MUST use the environment from `environ.FromCtx` when inspecting the
// environment or launching subprocesses. In order to ensure this, the process
// environment is cleared from the time that MainFn starts.
type MainFn func(ctx context.Context, build *Build, userargs []string) error

func splitArgs(args []string) ([]string, []string) {
	for i, arg := range args {
		if arg == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

func mkOutputHandler(ctx context.Context, exeArgs []string, build *Build) func() {
	fs := flag.NewFlagSet(exeArgs[0], flag.ExitOnError)
	outputFlag := luciexe.AddOutputFlagToSet(fs)
	fs.Parse(exeArgs[1:])
	if outputFlag.Codec.IsNoop() {
		return nil
	}
	return func() {
		build, _ := build.Detach(ctx)
		if err := outputFlag.Write(build); err != nil {
			panic(errors.Annotate(err, "writing final build").Err())
		}
	}
}

func buildFrom(in io.Reader, build *bbpb.Build) {
	data, err := ioutil.ReadAll(in)
	if err != nil {
		panic(errors.Annotate(err, "reading Build from stdin").Err())
	}
	if err := proto.Unmarshal(data, build); err != nil {
		panic(errors.Annotate(err, "parsing Build from stdin").Err())
	}
	// Initialize Output.Properties so that users can use exe.WriteProperties
	// straight away.
	if build.Output == nil {
		build.Output = &bbpb.Build_Output{}
	}
	if build.Output.Properties == nil {
		build.Output.Properties = &structpb.Struct{}
	}
}

func mkLogdogSink(ctx context.Context, build *bbpb.Build, cfg *config) func(*bbpb.Build) {
	if cfg.lim <= 0 {
		return nil
	}

	cType := luciexe.BuildProtoContentType
	if cfg.zlibLevel > 0 {
		cType = luciexe.BuildProtoZlibContentType
	}
	buildStream, err := cfg.ldClient.NewDatagramStream(
		ctx, luciexe.BuildProtoStreamSuffix,
		streamclient.WithContentType(cType))

	if err != nil {
		panic(err)
	}

	var buf *bytes.Buffer
	var z *zlib.Writer

	if cfg.zlibLevel > 0 {
		buf = &bytes.Buffer{}
		z, err = zlib.NewWriterLevel(buf, cfg.zlibLevel)
		if err != nil {
			panic(errors.Annotate(err, "unable to create zlib.Writer").Err())
		}
	}

	return func(build *bbpb.Build) {
		data, err := proto.Marshal(build)
		if err != nil {
			panic(errors.Annotate(err, "unable to marshal Build state").Err())
		}

		if buf != nil {
			buf.Reset()
			z.Reset(buf)
			if _, err := z.Write(data); err != nil {
				panic(errors.Annotate(err, "unable to write to zlib.Writer").Err())
			}
			if err := z.Close(); err != nil {
				panic(errors.Annotate(err, "unable to close zlib.Writer").Err())
			}
			data = buf.Bytes()
		}

		if err := buildStream.WriteDatagram(data); err != nil {
			panic(errors.Annotate(err, "unable to write Build state").Err())
		}
	}
}

// Run executes the `main` callback.
//
// To implement the luciexe protocol:
//
//   func main() {
//     exe.Run(func(ctx context.Context, input *exe.Build, userArgs []string) error {
//       ... do whatever you want here ...
//       return nil // nil error indicates successful build.
//     })
//   }
//
// This calls os.Exit on completion of `main`, or panics if something went
// wrong.
//
// If main panics, this is converted to an INFRA_FAILURE. Otherwise main's
// returned error is converted to a build status with GetErrorStatus.
func Run(main MainFn, options ...RunOption) {
	options = append(options, func(c *config) {
		if logging.GetFactory(c.ctx) == nil {
			c.ctx = gologger.StdConfig.Use(c.ctx)
		}
		if c.ldClient == nil {
			bs, err := bootstrap.GetFromEnv(environ.FromCtx(c.ctx))
			if err != nil {
				panic(errors.Annotate(err, "unable to initialize logdog client").Err())
			}
			c.ldClient = bs.Client
		}
	})

	os.Exit(runImpl(os.Args, options, main))
}

func runImpl(args []string, opts []RunOption, main MainFn) int {
	cfg := &config{
		ctx: environ.With(context.Background(), environ.System()),

		lim: rate.Every(time.Second),
	}
	os.Clearenv()

	for _, o := range opts {
		if o != nil {
			o(cfg)
		}
	}
	ctx := cfg.ctx

	exeArgs, userArgs := splitArgs(args)

	build := &bbpb.Build{}
	buildFrom(os.Stdin, build)

	sinkFn := mkLogdogSink(ctx, build, cfg)

	ctx, buildObj := SinkBuildUpdates(ctx, build, cfg.ldClient, cfg.lim, sinkFn)
	defer func() {
		// one final send; this ensures that the build is sent with a finalized
		// Status. mkLogdogSink generates sinkFn so that it will do one final send
		// when the context is canceled.
		if _, sendFn := buildObj.Detach(ctx); sendFn != nil {
			sendFn()
		}
	}()

	if handleFn := mkOutputHandler(ctx, exeArgs, buildObj); handleFn != nil {
		defer handleFn()
	}

	return runUserCode(ctx, buildObj, userArgs, main)
}

// runUserCode should convert all user code errors/panic's into non-panicing
// state in `build`.
func runUserCode(ctx context.Context, build *Build, userArgs []string, main MainFn) (retcode int) {
	finalize := func(err error) {
		b, _ := build.Detach(ctx)
		if protoutil.IsEnded(b.Status) {
			panic(errors.New("finalize called on finished Build"))
		}
		b.Status, b.StatusDetails = GetErrorStatus(err)
		b.EndTime = google.NewTimestamp(clock.Now(ctx))
	}

	defer func() {
		if errI := recover(); errI != nil {
			retcode = 2

			finalize(errors.New("panic", StatusInfraFailure))

			logging.Errorf(ctx, "main function paniced: %s", errI)
			if err, ok := errI.(error); ok {
				errors.Log(ctx, err)
			}
			logging.Errorf(ctx, "original traceback: %s", debug.Stack())
		}
	}()

	cCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	signals.HandleInterrupt(cancel)

	err := main(cCtx, build, userArgs)
	finalize(err)

	if err != nil {
		logging.Errorf(ctx, "main function failed: %s", err)
		errors.Log(ctx, err)
		retcode = 1
	}

	return
}
