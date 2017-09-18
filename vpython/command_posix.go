// Copyright 2017 The LUCI Authors.
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

// +build darwin linux freebsd netbsd openbsd android

package vpython

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"

	"golang.org/x/net/context"
)

// This is a generic system command set. Its purpose is to communicate
// information between spawning and receiving processes in an environment
// variable friendly way.
//
// At the moment it's only used in POSIX code, so we put it here. If other
// platforms ever need this capability, this code should be moved to
// "command.go".

func createCommand(c context.Context, name string, val interface{}, env environ.Env) (*exec.Cmd, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, errors.Annotate(err, "could not get executable").Err()
	}

	env = env.Clone()
	if err := setCommand(env, name, val); err != nil {
		return nil, err
	}

	// Start our monitoring process.
	cmd := exec.Command(self)
	cmd.Env = env.Sorted()
	cmd.Stderr = os.Stderr
	return cmd, nil
}

func setCommand(env environ.Env, name string, val interface{}) error {
	if val != nil {
		valEnc, err := envValueEncode(val)
		if err != nil {
			return err
		}
		name = fmt.Sprintf("%s:%s", name, valEnc)
	}
	env.Set(ApplicationCommandENV, name)
	return nil
}

func envValueEncode(obj interface{}) (string, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return "", errors.Annotate(err, "failed to marshal").Err()
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func envValueDecode(v string, obj interface{}) error {
	data, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return errors.Annotate(err, "failed to base64 decode").Err()
	}
	if err := json.Unmarshal(data, obj); err != nil {
		return errors.Annotate(err, "failed to unmarshal").Err()
	}
	return nil
}
