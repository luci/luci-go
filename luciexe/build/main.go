// Copyright 2020 The LUCI Authors.
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

package build

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"golang.org/x/time/rate"
	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/lucictx"

	"go.chromium.org/luci/luciexe"
)

var errNonSuccess = errors.New("build state != SUCCESS")

// Main implements all the 'command-line' behaviors of the luciexe 'exe'
// protocol, including:
//
//   - parsing command line for "--output", "--help", etc.
//   - parsing stdin (as appropriate) for the incoming Build message
//   - creating and configuring a logdog client to send State evolutions.
//   - Configuring a logdog client from the environment.
//   - Writing Build state updates to the logdog "build.proto" stream.
//   - Start'ing the build in this process.
//   - End'ing the build with the returned error from your function
//
// Input and Output properties can be manipulated by registering property
// schemas with the package-level Properties object.
//
// CLI Arguments parsed:
//   - -h / --help : Print help for this binary (including input/output
//     property type info)
//   - --output : luciexe "output" flag; See
//     https://pkg.go.dev/go.chromium.org/luci/luciexe#hdr-Recursive_Invocation
//   - --working-dir : The working directory to run from; Default is $PWD unless
//     LUCIEXE_FAKEBUILD is set, in which case a temp dir is used and cleaned up after.
//   - -- : Any extra arguments after a "--" token are passed to your callback
//     as-is.
//
// Example:
//
//	// MyProps will serve as both the input and output properties schema. If you
//	// want to have differing schemas, you can use RegisterSplitProperty. If you
//	// want to have Input-only or Output-only properties, see the
//	// Register{Input,Output}Property variants.
//	//
//	// See docs on RegisterProperty for more info.
//	var topLevelProps = build.RegisterProperty[*MyProps]("")
//
//	func main() {
//	  build.Main(func(ctx context.Context, args []string, st *build.State) error {
//	    // actual build code here, build is already Start'differing
//	    parsedInput := topLevelProps.GetInput(ctx) // returns *MyProps
//	    topLevelProps.MutateOutput(ctx, func(val *MyProps) (mutated bool) {
//	      // modify `val` however you like - if you need to copy some value from
//	      // it, use e.g. proto.Clone to copy it.
//	      return true
//	    })
//	    return nil // will mark the Build as SUCCESS
//	  })
//	}
//
// Main also supports running a luciexe build locally by specifying the
// LUCIEXE_FAKEBUILD environment variable. The value of the variable should
// be a path to a file containing a JSON-encoded Build proto.
//
// TODO(iannucci): LUCIEXE_FAKEBUILD does not support nested invocations.
// It should set up bbagent and butler in order to aggregate logs.
func Main(cb func(context.Context, []string, *State) error) {
	ctx := gologger.StdConfig.Use(context.Background())

	switch err := main(ctx, os.Args, os.Stdin, cb); err {
	case nil:
		os.Exit(0)

	case errNonSuccess:
		os.Exit(1)
	default:
		errors.Log(ctx, err)
		os.Exit(2)
	}
}

func main(ctx context.Context, args []string, stdin io.Reader, cb func(context.Context, []string, *State) error) (err error) {
	args = append([]string(nil), args...)
	var userArgs []string
	for i, a := range args {
		if a == "--" {
			userArgs = args[i+1:]
			args = args[:i]
			break
		}
	}

	opts := []StartOption{}

	outputFile, wd, help := parseArgs(args)
	if wd != "" {
		if err := os.Chdir(wd); err != nil {
			return err
		}
	}

	var initial *bbpb.Build
	var lastBuild *bbpb.Build

	defer func() {
		if outputFile != "" && lastBuild != nil {
			if err := luciexe.WriteBuildFile(outputFile, lastBuild); err != nil {
				logging.Errorf(ctx, "failed to write outputFile: %s", err)
			}
		}
	}()

	if !help {
		if path := environ.FromCtx(ctx).Get(luciexeFakeVar); path != "" {
			var cleanup func(*error)
			ctx, initial, cleanup, err = prepFromFakeLuciexeEnv(ctx, wd, path)
			if cleanup != nil {
				defer cleanup(&err)
			}
		} else {
			var moreOpts []StartOption
			initial, moreOpts, err = prepOptsFromLuciexeEnv(ctx, stdin, &lastBuild)
			if err != nil {
				return err
			}
			opts = append(opts, moreOpts...)
		}
	}

	state, ictx, err := Start(ctx, initial, opts...)
	if err != nil {
		return err
	}

	if help {
		logging.Infof(ctx, "`%s` is a `luciexe` binary. See go.chromium.org/luci/luciexe.", args[0])
		logging.Infof(ctx, "======= I/O Proto =======")

		buf := &bytes.Buffer{}
		state.SynthesizeIOProto(buf)
		logging.Infof(ctx, "%s", buf.Bytes())
		return nil
	}

	runUserCb(ictx, userArgs, state, cb)
	if state.buildPb.Output.Status != bbpb.Status_SUCCESS {
		return errNonSuccess
	}
	return nil
}

// overridden in tests
var mainSendRate = rate.Every(time.Second)

const luciexeFakeVar = "LUCIEXE_FAKEBUILD"

func prepFromFakeLuciexeEnv(ctx context.Context, wd, buildPath string) (newCtx context.Context, initial *bbpb.Build, cleanup func(*error), err error) {
	// This is a fake build.
	logging.Infof(ctx, "Running fake build because %s is set. Build proto path: %s", luciexeFakeVar, buildPath)

	// Pull the Build proto from the path in the environment variable and
	// set up the environment according to the LUCI User Code Contract.
	newCtx = ctx
	env := environ.FromCtx(ctx)

	// Defensively clear the environment variable so it doesn't propagate to subtasks.
	env.Remove(luciexeFakeVar)

	// Read the Build proto.
	initial, err = readLuciexeFakeBuild(buildPath)
	if err != nil {
		return
	}

	if wd == "" {
		// Create working directory heirarchy in a new temporary directory.
		wd, err = os.MkdirTemp("", fmt.Sprintf("luciexe-fakebuild-%s-", filepath.Base(os.Args[0])))
		if err != nil {
			return
		}
		err = os.Chdir(wd)
		if err != nil {
			return
		}
		cleanup = func(err *error) {
			logging.Infof(ctx, "Cleaning up working directory: %s", wd)
			if r := os.RemoveAll(wd); r != nil && *err == nil {
				*err = r
			} else if r != nil {
				logging.Errorf(ctx, "Failed to clean up working directory: %s", wd)
			}
		}
	}
	logging.Infof(ctx, "Working directory: %s", wd)

	tmpDir := filepath.Join(wd, "tmp")
	if err = os.MkdirAll(tmpDir, os.ModePerm); err != nil {
		return
	}
	cacheDir := filepath.Join(wd, "cache")
	if err = os.MkdirAll(cacheDir, os.ModePerm); err != nil {
		return
	}

	// Set up environment and LUCI_CONTEXT.
	for _, key := range luciexe.TempDirEnvVars {
		env.Set(key, tmpDir)
	}
	newCtx = env.SetInCtx(newCtx)
	newCtx = lucictx.SetLUCIExe(newCtx, &lucictx.LUCIExe{
		CacheDir: cacheDir,
	})

	return
}

func prepOptsFromLuciexeEnv(ctx context.Context, stdin io.Reader, lastBuild **bbpb.Build) (initial *bbpb.Build, opts []StartOption, err error) {
	initial, err = readStdinBuild(stdin)
	if err != nil {
		return
	}

	bs, err := bootstrap.GetFromEnv(environ.FromCtx(ctx))
	if err != nil {
		return
	}

	buildStream, err := bs.Client.NewDatagramStream(
		ctx, "build.proto",
		streamclient.WithContentType(luciexe.BuildProtoZlibContentType))
	if err != nil {
		return
	}

	zlibBuf := &bytes.Buffer{}
	zlibWriter := zlib.NewWriter(zlibBuf)

	opts = append(opts,
		OptLogsink(bs.Client),
		OptSend(mainSendRate, func(vers int64, build *bbpb.Build) {
			*lastBuild = build
			data, err := proto.Marshal(build)
			if err != nil {
				panic(err)
			}

			zlibBuf.Reset()
			zlibWriter.Reset(zlibBuf)

			if _, err := zlibWriter.Write(data); err != nil {
				panic(err)
			}
			if err := zlibWriter.Close(); err != nil {
				panic(err)
			}
			if err := buildStream.WriteDatagram(zlibBuf.Bytes()); err != nil {
				panic(err)
			}
		}),
	)

	return
}

func runUserCb(ctx context.Context, userArgs []string, state *State, cb func(context.Context, []string, *State) error) {
	var err error
	defer func() {
		state.End(err)
		if thing := recover(); thing != nil {
			logging.Errorf(ctx, "recovered panic: %s", thing)
			logging.Errorf(ctx, "traceback:\n%s", debug.Stack())
		}
	}()
	err = cb(ctx, userArgs, state)
}

func readStdinBuild(stdin io.Reader) (*bbpb.Build, error) {
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, errors.Annotate(err, "reading *Build from stdin").Err()
	}

	ret := &bbpb.Build{}
	err = proto.Unmarshal(data, ret)
	return ret, err
}

func readLuciexeFakeBuild(filename string) (*bbpb.Build, error) {
	// N.B. This proto is JSON-encoded.
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, errors.Annotate(err, "reading *Build from %s=%s", luciexeFakeVar, filename).Err()
	}
	ret := &bbpb.Build{}
	err = json.Unmarshal(data, ret)
	if err != nil {
		return nil, errors.Annotate(err, "decoding *Build from %s=%s", luciexeFakeVar, filename).Err()
	}
	return ret, nil
}

func parseArgs(args []string) (output, wd string, help bool) {
	fs := flag.FlagSet{}
	fs.StringVar(&wd, "working-dir", "", "The working directory to run from; Default is $PWD unless LUCIEXE_FAKEBUILD is set, in which case a temp dir is used and cleaned up after.")
	fs.StringVar(&output, "output", "", "Output final Build message to this path. Must end with {.json,.pb,.textpb}")
	fs.BoolVar(&help, "help", false, "Print help for this executable")
	fs.BoolVar(&help, "h", false, "Print help for this executable")
	if err := fs.Parse(args[1:]); err != nil {
		panic(err)
	}
	return
}
