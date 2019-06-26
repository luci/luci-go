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

package luciexe

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/lucictx"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func parseArgs(args []string) (*pb.RunnerArgs, error) {
	fs := flag.FlagSet{}
	argsB64 := fs.String("args-b64gz", "", text.Doc(`
		(standard raw, unpadded base64)-encoded,
		zlib-compressed,
		binary buildbucket.v2.RunnerArgs protobuf message.
	`))
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	ann := func(err error) error {
		return errors.Annotate(err, "-args-b64gz").Err()
	}

	if *argsB64 == "" {
		return nil, ann(errors.Reason("required").Err())
	}

	compressed, err := base64.RawStdEncoding.DecodeString(*argsB64)
	if err != nil {
		return nil, ann(err)
	}

	decompressing, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, ann(err)
	}
	decompressed, err := ioutil.ReadAll(decompressing)
	if err != nil {
		return nil, ann(err)
	}

	ret := &pb.RunnerArgs{}
	if err := proto.Unmarshal(decompressed, ret); err != nil {
		return nil, ann(err)
	}
	return ret, nil
}

// readBuildSecrets reads BuildSecrets message from swarming secret bytes.
func readBuildSecrets(ctx context.Context) (*pb.BuildSecrets, error) {
	swarming := lucictx.GetSwarming(ctx)
	if swarming == nil {
		return nil, errors.Reason("no swarming secret bytes; is this a Swarming Task with secret bytes?").Err()
	}

	secrets := &pb.BuildSecrets{}
	if err := proto.Unmarshal(swarming.SecretBytes, secrets); err != nil {
		return nil, errors.Annotate(err, "failed to read BuildSecrets message from swarming secret bytes").Err()
	}
	return secrets, nil
}

// RunnerMain runs LUCI runner, a program that runs a LUCI executable.
func RunnerMain(args []string) int {
	if err := mainErr(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func mainErr(rawArgs []string) error {
	ctx := context.Background()
	ctx = gologger.StdConfig.Use(ctx)

	args, err := parseArgs(rawArgs)
	if err != nil {
		return err
	}

	secrets, err := readBuildSecrets(ctx)
	if err != nil {
		return err
	}

	client := newBuildsClient(args)

	r := &runner{
		UpdateBuild: func(ctx context.Context, req *pb.UpdateBuildRequest) error {
			// Insert the build token into the context.
			ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(buildbucket.BuildTokenHeader, secrets.BuildToken))
			_, err := client.UpdateBuild(ctx, req)
			return err
		},
	}
	return r.Run(ctx, args)
}

// newBuildsClient creates a buildbucket client.
func newBuildsClient(args *pb.RunnerArgs) pb.BuildsClient {
	opts := prpc.DefaultOptions()
	opts.Insecure = lhttp.IsLocalHost(args.BuildbucketHost)
	opts.Retry = nil // luciexe handles retries itself.

	return pb.NewBuildsPRPCClient(&prpc.Client{
		Host:    args.BuildbucketHost,
		Options: opts,
	})
}
