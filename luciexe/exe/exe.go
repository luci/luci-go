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
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/luciexe"
)

type runMiddle func(ctx context.Context, cfg *config, initial *bbpb.Build, userArgs []string, sendFn func(*bbpb.Build) error) (*bbpb.Build, interface{}, error)

func splitArgs(args []string) ([]string, []string) {
	for i, arg := range args {
		if arg == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

func handleOutput(ctx context.Context, exeArgs []string, build *bbpb.Build) {
	fs := flag.NewFlagSet(exeArgs[0], flag.ExitOnError)
	outputFlag := luciexe.AddOutputFlagToSet(fs)
	fs.Parse(exeArgs[1:])
	if outputFlag.Codec.IsNoop() {
		return
	}
	if err := outputFlag.Write(build); err != nil {
		panic(errors.Annotate(err, "writing final build").Err())
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

func mkLogdogSink(ctx context.Context, build *bbpb.Build, cfg *config) func(*bbpb.Build) error {
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

	return func(build *bbpb.Build) error {
		// NOTE: we panic in this function because any of the errors here mean that
		// we've desynchronized with the logdog stream; In that event we don't have
		// enough information to recover or retry. Crashing loudly is the best thing
		// to do.

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

		return nil
	}
}

func runImpl(args []string, opts []RunOption, middle runMiddle) int {
	opts = append(opts, func(c *config) {
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

	rawBuild := &bbpb.Build{}
	buildFrom(os.Stdin, rawBuild)

	sendFn := mkLogdogSink(ctx, rawBuild, cfg)

	lastBuild, recovered, err := middle(ctx, cfg, rawBuild, userArgs, sendFn)

	handleOutput(ctx, exeArgs, lastBuild)

	switch {
	case recovered != nil:
		return 2
	case err != nil:
		return 1
	default:
		return 0
	}
}
