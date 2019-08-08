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

package runner

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/lucictx"

	pb "go.chromium.org/luci/buildbucket/proto"
)

type workdir struct {
	authDir    string
	logdogDir  string
	luciCtxDir string

	// This is the CWD for the user executable itself. Keep it short. This
	// is important to allow tasks on Windows to have as many path characters as
	// possible; otherwise they run into MAX_PATH issues.
	userDir string

	// We can't use a subdir of userDir because some overzealous scripts like to
	// remove everything they find under TEMPDIR.
	userTempDir string
}

func mkAbs(base, path string) (string, error) {
	ret, err := filepath.Abs(filepath.Join(base, path))
	if err != nil {
		return "", errors.Annotate(err, "filepath.Abs(%q)", path).Err()
	}
	if err = os.Mkdir(ret, 0700); err != nil {
		return "", errors.Annotate(err, "os.Mkdir(%q)", path).Err()
	}
	return ret, nil
}

func (w *workdir) prep(base string) error {
	var err error
	if w.authDir, err = mkAbs(base, "auth-configs"); err != nil {
		return err
	}
	if w.logdogDir, err = mkAbs(base, "logdog"); err != nil {
		return err
	}
	if w.luciCtxDir, err = mkAbs(base, "luci-context"); err != nil {
		return err
	}
	if w.userDir, err = mkAbs(base, "u"); err != nil {
		return err
	}
	if w.userTempDir, err = mkAbs(base, "ut"); err != nil {
		return err
	}
	return nil
}

func parseRunnerArgs(encodedData string) (*pb.RunnerArgs, error) {
	if encodedData == "" {
		return nil, errors.Reason("required").Err()
	}

	compressed, err := base64.RawStdEncoding.DecodeString(encodedData)
	if err != nil {
		return nil, err
	}

	decompressing, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	decompressed, err := ioutil.ReadAll(decompressing)
	if err != nil {
		return nil, err
	}

	ret := &pb.RunnerArgs{}
	return ret, proto.Unmarshal(decompressed, ret)
}

func normalizeRunnerArgs(args *pb.RunnerArgs) error {
	switch {
	case args.BuildbucketHost == "":
		return errors.Reason("buildbucket_host is required").Err()
	case args.LogdogHost == "":
		return errors.Reason("logdog_host is required").Err()
	case args.Build.GetId() == 0:
		return errors.Reason("build.id is required").Err()
	}

	normalizePath := func(title, path string) (string, error) {
		if path == "" {
			return "", errors.Reason("%s is required", title).Err()
		}
		return filepath.Abs(path)
	}

	var err error
	if args.ExecutablePath, err = normalizePath("executable_path", args.ExecutablePath); err != nil {
		return err
	}
	if args.CacheDir, err = normalizePath("cache_dir", args.CacheDir); err != nil {
		return err
	}
	return nil
}

func parseArgs(args []string) (ret *pb.RunnerArgs, wkDir workdir, err error) {
	fs := flag.FlagSet{}
	argsB64 := fs.String("args-b64gz", "", text.Doc(`
		(standard raw, unpadded base64)-encoded,
		zlib-compressed,
		binary buildbucket.v2.RunnerArgs protobuf message.
	`))
	if err = fs.Parse(args); err != nil {
		return
	}

	if ret, err = parseRunnerArgs(*argsB64); err != nil {
		err = errors.Annotate(err, "-args-b64gz").Err()
		return
	}

	// Validate and normalize parameters.
	if err = normalizeRunnerArgs(ret); err != nil {
		err = errors.Annotate(err, "normalizing args").Err()
		return
	}

	err = wkDir.prep(".")
	return
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

// Main runs LUCI runner, a program that runs a LUCI executable.
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

	args, wkDir, err := parseArgs(rawArgs[1:])
	if err != nil {
		return err
	}
	logging.Infof(ctx, "RunnerArgs: %s", indentedJSONPB(args))

	secrets, err := readBuildSecrets(ctx)
	if err != nil {
		return err
	}

	client := newBuildsClient(args)

	return run(ctx, args, wkDir, func(ctx context.Context, req *pb.UpdateBuildRequest) error {
		// Insert the build token into the context.
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(buildbucket.BuildTokenHeader, secrets.BuildToken))
		_, err := client.UpdateBuild(ctx, req)
		return err
	})
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
