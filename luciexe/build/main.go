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
	"flag"
	"io"
	"io/ioutil"
	"os"
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
	"go.chromium.org/luci/luciexe"
)

var errNonSuccess = errors.New("build state != SUCCESS")

// Main implements all the 'command-line' behaviors of the luciexe 'exe'
// protocol, including:
//
//   * parsing command line for "--output", "--help", etc.
//   * parsing stdin (as appropriate) for the incoming Build message
//   * creating and configuring a logdog client to send State evolutions.
//   * Configuring a logdog client from the environment.
//   * Writing Build state updates to the logdog "build.proto" stream.
//   * Start'ing the build in this process.
//   * End'ing the build with the returned error from your function
//
// If `inputMsg` is nil, the top-level properties will be ignored.
//
// If `writeFnptr` and `mergeFnptr` are nil, they're ignored. Otherwise
// they work as they would for MakePropertyModifier.
//
// CLI Arguments parsed:
//   * -h / --help : Print help for this binary (including input/output
//     property type info)
//   * --strict-input : Enable strict property parsing (see OptStrictInputProperties)
//   * --output : luciexe "output" flag; See
//     https://pkg.go.dev/go.chromium.org/luci/luciexe#hdr-Recursive_Invocation
//   * -- : Any extra arguments after a "--" token are passed to your callback
//     as-is.
//
// Example:
//
//    func main() {
//      input := *MyInputProps{}
//      var writeOutputProps func(*MyOutputProps)
//      var mergeOutputProps func(*MyOutputProps)
//
//      Main(input, &writeOutputProps, &mergeOutputProps, func(ctx context.Context, args []string, st *build.State) error {
//        // actual build code here, build is already Start'd
//        // input was parsed from build.Input.Properties
//        writeOutputProps(&MyOutputProps{...})
//        return nil // will mark the Build as SUCCESS
//      })
//    }
//
// NOTE: These types are pretty bad; There's significant opportunity to improve
// them with Go2 generics.
func Main(inputMsg proto.Message, writeFnptr, mergeFnptr interface{}, cb func(context.Context, []string, *State) error) {
	ctx := gologger.StdConfig.Use(context.Background())

	switch err := main(ctx, os.Args, os.Stdin, inputMsg, writeFnptr, mergeFnptr, cb); err {
	case nil:
		os.Exit(0)

	case errNonSuccess:
		os.Exit(1)
	default:
		errors.Log(ctx, err)
		os.Exit(2)
	}
}

func main(ctx context.Context, args []string, stdin io.Reader, inputMsg proto.Message, writeFnptr, mergeFnptr interface{}, cb func(context.Context, []string, *State) error) error {
	args = append([]string(nil), args...)
	var userArgs []string
	for i, a := range args {
		if a == "--" {
			userArgs = args[i+1:]
			args = args[:i]
			break
		}
	}

	opts := []StartOption{
		OptParseProperties(inputMsg),
	}
	if writeFnptr != nil || mergeFnptr != nil {
		opts = append(opts, OptOutputProperties(writeFnptr, mergeFnptr))
	}

	outputFile, strict, help := parseArgs(args)
	if strict {
		opts = append(opts, OptStrictInputProperties())
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

	var err error

	if !help {
		var moreOpts []StartOption
		initial, moreOpts, err = prepOptsFromLuciexeEnv(ctx, stdin, &lastBuild)
		if err != nil {
			return err
		}
		opts = append(opts, moreOpts...)
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
	if state.buildPb.Status != bbpb.Status_SUCCESS {
		return errNonSuccess
	}
	return nil
}

// overridden in tests
var mainSendRate = rate.Every(time.Second)

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
	data, err := ioutil.ReadAll(stdin)
	if err != nil {
		return nil, errors.Annotate(err, "reading *Build from stdin").Err()
	}

	ret := &bbpb.Build{}
	err = proto.Unmarshal(data, ret)
	return ret, err
}

func parseArgs(args []string) (output string, strict bool, help bool) {
	fs := flag.FlagSet{}
	fs.BoolVar(&strict, "strict-input", false, "Strict input parsing; Input properties supplied which aren't understood by this program will print an error and quit.")
	fs.StringVar(&output, "output", "", "Output final Build message to this path. Must end with {.json,.pb,.textpb}")
	fs.BoolVar(&help, "help", false, "Print help for this executable")
	fs.BoolVar(&help, "h", false, "Print help for this executable")
	if err := fs.Parse(args[1:]); err != nil {
		panic(err)
	}
	return
}
