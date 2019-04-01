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

package normalize

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/golang/protobuf/proto"

	pb "go.chromium.org/luci/buildbucket/proto/config"
	"go.chromium.org/luci/common/logging"
)

const (
	flattenerVersion = "git_revision:1eb070139f88270631d9017ce9798853792dbd20"
)

// Buildbucket normalizes cr-buildbucket.cfg config.
func Buildbucket(c context.Context, cfg *pb.BuildbucketCfg) error {
	// Install or update 'flatten_buildbucket_cfg' tool.
	bin, err := installFlattenBuildbucketCfg(c)
	if err != nil {
		return fmt.Errorf("failed to install buildbucket config flattener: %s", err)
	}

	// 'flatten_buildbucket_cfg' wants a real file as input.
	f, err := ioutil.TempFile("", "lucicfg")
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		os.Remove(f.Name())
	}()
	if err := proto.MarshalText(f, cfg); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	buf := bytes.Buffer{}

	cmd := exec.Command(bin, f.Name())
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to flatten the config - %s", err)
	}

	*cfg = pb.BuildbucketCfg{}
	if err := proto.UnmarshalText(buf.String(), cfg); err != nil {
		return err
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////

var flattenerPath = ""

func installFlattenBuildbucketCfg(c context.Context) (bin string, err error) {
	// Do not install twice, just a waste of time resolving 'latest'.
	if flattenerPath != "" {
		return flattenerPath, nil
	}

	// Install into TMP/flatten_buildbucket_cfg and hope TMP is mounted as
	// executable...
	dest := filepath.Join(os.TempDir(), "flatten_buildbucket_cfg")
	logging.Infof(c, "Installing infra/tools/flatten_buildbucket_cfg into %s", dest)

	cmd := exec.Command("cipd", "ensure", "-root", dest, "-ensure-file", "-")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = strings.NewReader("infra/tools/flatten_buildbucket_cfg " + flattenerVersion)
	if err = cmd.Run(); err != nil {
		return
	}

	flattenerPath = filepath.Join(dest, "flatten_buildbucket_cfg")
	return flattenerPath, nil
}
