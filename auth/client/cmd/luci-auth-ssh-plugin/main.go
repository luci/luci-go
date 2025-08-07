// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Command luci-auth-ssh-plugin is an auth plugin for git-credential-luci.
//
// See git-credential-luci for info on how to use auth plugins.
//
// This plugin supports proxying the authentication over an SSH
// connection via the SSH agent channel.  This plugin is run on the
// remote host, and it forwards auth requests to a local helper that
// piggybacks on the SSH agent.
package main

import (
	"context"
	"os"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
)

func main() {
	ctx := gologger.StdConfig.Use(context.Background())
	ctx = logging.SetLevel(ctx, logging.Info)
	sshPluginMain(
		ctx,
		newDefaultAgentDialer(),
		os.Stdin,
		os.Stdout,
	)
}
