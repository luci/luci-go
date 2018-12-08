// Copyright 2018 The LUCI Authors.
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

package main

import (
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"strconv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// Ensure UpstartStrategy implements PlatformStrategy.
var _ PlatformStrategy = &UpstartStrategy{}

// LinuxStrategy is a Linux-specific partial PlatformStrategy.
// Does not fully implement PlatformStrategy.
type LinuxStrategy struct {
}

// chown modifies the given path to be owned by the given user.
// Implements PlatformStrategy.
func (*LinuxStrategy) chown(c context.Context, path, username string) error {
	u, err := user.Lookup(username)
	if err != nil {
		return errors.Annotate(err, "failed to look up local user %q", username).Err()
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return errors.Annotate(err, "failed to get uid for user %q", username).Err()
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return errors.Annotate(err, "failed to get gid for user %q", username).Err()
	}
	return os.Chown(path, uid, gid)
}

// UpstartStrategy is a Linux-specific PlatformStrategy using upstart.
// Implements PlatformStrategy.
type UpstartStrategy struct {
	LinuxStrategy
}

// upstartCfg is the path to write the upstart config.
const upstartCfg = "/etc/init/swarming-start-bot.conf"

// upstartTmpl is the name of the upstart config template asset.
const upstartTmpl = "swarming-start-bot.conf.tmpl"

// upstartSrv is the name of the upstart service.
const upstartSrv = "swarming-start-bot"

// autostart configures the given Swarming bot code to be executed on startup for the given user,
// then starts the Swarming bot process.
// Implements PlatformStrategy.
func (*UpstartStrategy) autostart(c context.Context, path, user string) error {
	subs := map[string]string{
		"BotCode": path,
		"User":    user,
	}
	s, err := substitute(c, string(GetAsset(upstartTmpl)), subs)
	if err != nil {
		return errors.Annotate(err, "failed to prepare template %q", upstartTmpl).Err()
	}

	logging.Infof(c, "installing: %s", upstartCfg)
	// 0644 allows the upstart config to be read by all users.
	// Useful when SSHing to the instance.
	if err := ioutil.WriteFile(upstartCfg, []byte(s), 0644); err != nil {
		return errors.Annotate(err, "failed to write: %s", upstartCfg).Err()
	}

	logging.Infof(c, "starting %q", upstartSrv)
	if err := exec.Command("initctl", "start", upstartSrv).Run(); err != nil {
		return errors.Annotate(err, "failed to start service %q", upstartSrv).Err()
	}
	return nil
}

// newStrategy returns a new Linux-specific PlatformStrategy.
func newStrategy() PlatformStrategy {
	// TODO(smut): Support systemd.
	return &UpstartStrategy{}
}
