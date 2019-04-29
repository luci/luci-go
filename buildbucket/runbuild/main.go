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

package runbuild

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

	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging/gologger"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func parseArgs(args []string) (*pb.RunBuildArgs, error) {
	fs := flag.FlagSet{}
	argsB64 := fs.String("args-b64gz", "", text.Doc(`
		(standard raw, unpadded base64)-encoded,
		zlib-compressed,
		binary buildbucket.v2.RunBuildArgs protobuf message.
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

	ret := &pb.RunBuildArgs{}
	if err := proto.Unmarshal(decompressed, ret); err != nil {
		return nil, ann(err)
	}
	return ret, nil
}

// Main runs a build runner, a program that bootstraps a user executable.
func Main(args []string) int {
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

	_, err = (&buildRunner{}).Run(ctx, args)
	return err
}
