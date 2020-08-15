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
	"time"

	"github.com/golang/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"golang.org/x/time/rate"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/luciexe"
)

const (
	// ArgsDelim separates args for user program from the args needed by this
	// luciexe wrapper (e.g. `--output` flag). All args provided after the
	// first ArgsDelim will passed to user program.
	ArgsDelim = "--"
)

// MainFn is the function signature you must implement in your callback to Run.
//
// Args:
//  - ctx: The context will be canceled when the program receives the os
//   Interrupt or SIGTERM (on unix) signal. The context also has standard go
//   logging setup.
//  - build: The Build state, initialized with the Build read from stdin.
//  - userArgs: All command line arguments supplied after first `ArgsDelim`.
type MainFn func(ctx context.Context, build *Build, userargs []string) error

func splitArgs(args []string) ([]string, []string) {
	for i, arg := range args {
		if arg == ArgsDelim {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

func mkOutputHandler(ctx context.Context, exeArgs []string) func() {
	fs := flag.NewFlagSet(exeArgs[0], flag.ExitOnError)
	outputFlag := luciexe.AddOutputFlagToSet(fs)
	fs.Parse(exeArgs[1:])
	if outputFlag.Codec.IsNoop() {
		return nil
	}
	return func() {
		build, _ := getBuild(ctx).snapshot(-1)
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
	buildStream, err := getLogdogClient(ctx).Client.NewDatagramStream(
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

	nop := false
	limiter := rate.NewLimiter(cfg.lim, 1)

	return func(build *bbpb.Build) {
		if nop {
			return
		}

		if err := limiter.Wait(ctx); err != nil {
			nop = true
			if err == ctx.Err() {
				// if it's from the context error, we do one more send, then close the
				// stream.
				defer func() {
					if err := buildStream.Close(); err != nil {
						logging.Errorf(ctx, "closing build.proto stream: %s", err)
					}
				}()
			} else {
				logging.Errorf(ctx, "got error from lim.Wait: %s", err)
				return
			}
		}

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

// Run executes the `main` callback with a basic Context.
//
// This calls os.Exit on completion of `main`, or panics if something went
// wrong. If main panics, this is converted to an INFRA_FAILURE. If main returns
// a non-nil error, this is converted to FAILURE, unless the InfraErrorTag is
// set on the error (in which case it's converted to INFRA_FAILURE).
func Run(main MainFn, options ...Option) {
	ctx := gologger.StdConfig.Use(context.Background())
	ldc, err := bootstrap.Get()
	if err != nil {
		panic(errors.Annotate(err, "unable to initialize logdog client").Err())
	}

	ctx = setLogdogClient(ctx, ldc)
	os.Exit(runCtx(ctx, os.Args, options, main))
}

func runCtx(ctx context.Context, args []string, opts []Option, main MainFn) int {
	cfg := &config{
		lim: rate.Every(time.Second),
	}
	for _, o := range opts {
		if o != nil {
			o(cfg)
		}
	}
	exeArgs, userArgs := splitArgs(args)

	build := &bbpb.Build{}
	buildFrom(os.Stdin, build)

	logdogCtx, ldCancel := context.WithCancel(ctx)
	sinkFn := mkLogdogSink(logdogCtx, build, cfg)

	sinkCtx, sinkCancel := context.WithCancel(ctx)
	ctx, buildObj := SinkBuildUpdates(sinkCtx, build, sinkFn)
	defer func() {
		sinkCancel()
		buildObj.Wait()

		// one final send; this ensures that the build is sent with a finalized
		// Status. mkLogdogSink generates sinkFn so that it will do one final send
		// when the context is canceled.
		ldCancel()
		if sinkFn != nil {
			sinkFn(buildObj.build)
		}
	}()

	// mkOutputHandler relies on `ctx` containing the BuildObj.
	if handleFn := mkOutputHandler(ctx, exeArgs); handleFn != nil {
		defer handleFn()
	}

	return runUserCode(ctx, buildObj, userArgs, main)
}

// runUserCode should convert all user code errors/panic's into non-panicing
// state in `build`.
func runUserCode(ctx context.Context, build *Build, userArgs []string, main MainFn) (retcode int) {
	defer func() {
		if errI := recover(); errI != nil {
			retcode = 2
			build.Finalize(errors.New("panic", StatusInfraFailure))
			logging.Errorf(ctx, "main function paniced: %s", errI)
			if err, ok := errI.(error); ok {
				errors.Log(ctx, err)
			}
		}
	}()

	cCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	signals.HandleInterrupt(cancel)

	err := main(cCtx, build, userArgs)
	build.Finalize(err)

	if err != nil {
		logging.Errorf(ctx, "main function failed: %s", err)
		errors.Log(ctx, err)
		retcode = 1
	}

	return
}
