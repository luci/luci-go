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
	"fmt"
	"io"
	"os"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/environ"
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

// BuildSender is a function which may be called within the callback of Run to
// update this program's Build state.
//
// This function is bound to the Build message given to the `main` callback of
// Run.
//
// Panics if it cannot send the Build (which is never expected in normal
// operation).
type BuildSender func()

// InfraErrorTag should be set on errors returned from the `main` callback of
// Run.
//
// Errors with this tag set will cause the overall build status to be
// INFRA_FAILURE instead of FAILURE.
var InfraErrorTag = errtag.Make("infra_error", true)

// MainFn is the function signature you must implement in your callback to Run.
//
// Args:
//   - ctx: The context will be canceled when the program receives the os
//     Interrupt or SIGTERM (on unix) signal. The context also has standard go
//     logging setup.
//   - input: The initial Build state, as read from stdin. The build is not
//     protected by a mutex of any sort, so the `MainFn` is responsible
//     for protecting it if it can be modified from multiple goroutines.
//   - userArgs: All command line arguments supplied after first `ArgsDelim`.
//   - send: A send func which should be called after modifying the provided
//     build. The BuildSender is synchronous and locked; it may only be called
//     once at a time. It will marshal the current build, then send it. Writes
//     to the build should be synchronized with calls to the BuildSender.
//
// input.Output.Properties is initialized to an empty Struct so you can use
// WriteProperties right away.
type MainFn func(ctx context.Context, input *bbpb.Build, userargs []string, send BuildSender) error

func splitArgs(args []string) ([]string, []string) {
	for i, arg := range args {
		if arg == ArgsDelim {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

func mkOutputHandler(exeArgs []string, build *bbpb.Build) func() {
	fs := flag.NewFlagSet(exeArgs[0], flag.ExitOnError)
	outputFlag := luciexe.AddOutputFlagToSet(fs)
	fs.Parse(exeArgs[1:])
	if outputFlag.Codec.IsNoop() {
		return nil
	}
	return func() {
		if err := outputFlag.Write(build); err != nil {
			panic(errors.Fmt("writing final build: %w", err))
		}
	}
}

func buildFrom(in io.Reader, build *bbpb.Build) {
	data, err := io.ReadAll(in)
	if err != nil {
		panic(errors.Fmt("reading Build from stdin: %w", err))
	}
	if err := proto.Unmarshal(data, build); err != nil {
		panic(errors.Fmt("parsing Build from stdin: %w", err))
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

func mkBuildStream(ctx context.Context, build *bbpb.Build, zlibLevel int) (BuildSender, func() error) {
	bs, err := bootstrap.GetFromEnv(environ.FromCtx(ctx))
	if err != nil {
		panic(errors.Fmt("unable to make Logdog Client: %w", err))
	}

	cType := luciexe.BuildProtoContentType
	if zlibLevel > 0 {
		cType = luciexe.BuildProtoZlibContentType
	}
	buildStream, err := bs.Client.NewDatagramStream(
		ctx, luciexe.BuildProtoStreamSuffix,
		streamclient.WithContentType(cType))
	if err != nil {
		panic(err)
	}

	// TODO(iannucci): come up with a better API for this?
	var sendBuildMu sync.Mutex
	var buf *bytes.Buffer
	var z *zlib.Writer

	if zlibLevel > 0 {
		buf = &bytes.Buffer{}
		z, err = zlib.NewWriterLevel(buf, zlibLevel)
		if err != nil {
			panic(errors.Fmt("unable to create zlib.Writer: %w", err))
		}
	}

	return func() {
		sendBuildMu.Lock()
		defer sendBuildMu.Unlock()

		data, err := proto.Marshal(build)
		if err != nil {
			panic(errors.Fmt("unable to marshal Build state: %w", err))
		}

		if buf != nil {
			buf.Reset()
			z.Reset(buf)
			if _, err := z.Write(data); err != nil {
				panic(errors.Fmt("unable to write to zlib.Writer: %w", err))
			}
			if err := z.Close(); err != nil {
				panic(errors.Fmt("unable to close zlib.Writer: %w", err))
			}
			data = buf.Bytes()
		}

		if err := buildStream.WriteDatagram(data); err != nil {
			panic(errors.Fmt("unable to write Build state: %w", err))
		}
	}, buildStream.Close
}

// Run executes the `main` callback with a basic Context.
//
// This calls os.Exit on completion of `main`, or panics if something went
// wrong. If main panics, this is converted to an INFRA_FAILURE. If main returns
// a non-nil error, this is converted to FAILURE, unless the InfraErrorTag is
// set on the error (in which case it's converted to INFRA_FAILURE).
func Run(main MainFn, options ...Option) {
	os.Exit(runCtx(gologger.StdConfig.Use(context.Background()), os.Args, options, main))
}

func appendError(build *bbpb.Build, flavor string, errlike any) {
	if build.SummaryMarkdown != "" {
		build.SummaryMarkdown += "\n\n"
	}
	build.SummaryMarkdown += fmt.Sprintf("Final %s: %s", flavor, errlike)
	if build.Output == nil {
		build.Output = &bbpb.Build_Output{}
	}
	build.Output.SummaryMarkdown = build.SummaryMarkdown
}

func runCtx(ctx context.Context, args []string, opts []Option, main MainFn) int {
	cfg := &config{}
	for _, o := range opts {
		if o != nil {
			o(cfg)
		}
	}
	exeArgs, userArgs := splitArgs(args)

	build := &bbpb.Build{}
	if handleFn := mkOutputHandler(exeArgs, build); handleFn != nil {
		defer handleFn()
	}

	buildFrom(os.Stdin, build)
	sendBuild, closer := mkBuildStream(ctx, build, cfg.zlibLevel)
	defer func() {
		if err := closer(); err != nil {
			panic(err)
		}
	}()
	defer sendBuild()

	return runUserCode(ctx, build, userArgs, sendBuild, main)
}

// runUserCode should convert all user code errors/panic's into non-panicing
// state in `build`.
func runUserCode(ctx context.Context, build *bbpb.Build, userArgs []string, sendBuild BuildSender, main MainFn) (retcode int) {
	defer func() {
		if errI := recover(); errI != nil {
			retcode = 2
			build.Status = bbpb.Status_INFRA_FAILURE
			build.Output.Status = bbpb.Status_INFRA_FAILURE
			appendError(build, "panic", errI)
			logging.Errorf(ctx, "main function paniced: %s", errI)
			if err, ok := errI.(error); ok {
				errors.Log(ctx, err)
			}
		}
	}()

	cCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	signals.HandleInterrupt(cancel)
	if build.Output == nil {
		build.Output = &bbpb.Build_Output{}
	}
	if err := main(cCtx, build, userArgs, sendBuild); err != nil {
		if InfraErrorTag.In(err) {
			build.Status = bbpb.Status_INFRA_FAILURE
			build.Output.Status = bbpb.Status_INFRA_FAILURE
			appendError(build, "infra error", err)
		} else {
			build.Status = bbpb.Status_FAILURE
			build.Output.Status = bbpb.Status_FAILURE
			appendError(build, "error", err)
		}
		logging.Errorf(ctx, "main function failed: %s", err)
		errors.Log(ctx, err)
		retcode = 1
	} else {
		if !protoutil.IsEnded(build.Status) {
			build.Status = bbpb.Status_SUCCESS
		}
		if !protoutil.IsEnded(build.Output.Status) {
			build.Output.Status = build.Status
		}
		if build.Output.SummaryMarkdown == "" {
			build.Output.SummaryMarkdown = build.SummaryMarkdown
		}
	}
	return
}
