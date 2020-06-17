// Copyright 2015 The LUCI Authors.
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

package lib

import (
	"context"
	"fmt"
	"net/http"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/cipd/version"
	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/runtime/profiling"
)

type commonFlags struct {
	subcommands.CommandRunBase
	defaultFlags   common.Flags
	isolatedFlags  isolatedclient.Flags
	authFlags      authcli.Flags
	parsedAuthOpts auth.Options
	profilerFlags  profiling.Profiler
}

func (c *commonFlags) Init(authOpts auth.Options) {
	c.defaultFlags.Init(&c.Flags)
	c.isolatedFlags.Init(&c.Flags)
	c.authFlags.Register(&c.Flags, authOpts)
	c.profilerFlags.AddFlags(&c.Flags)
}

func (c *commonFlags) Parse() error {
	var err error
	if err = c.defaultFlags.Parse(); err != nil {
		return err
	}
	if err = c.isolatedFlags.Parse(); err != nil {
		return err
	}
	if err := c.profilerFlags.Start(); err != nil {
		return err
	}
	c.parsedAuthOpts, err = c.authFlags.Options()
	return err
}

func (c *commonFlags) createIsolatedClient(ctx context.Context, opts CommandOptions) (isolCl *isolatedclient.Client, err error) {
	var authCl *http.Client
	if opts.AuthClient != nil {
		authCl = opts.AuthClient
	} else {
		// Don't enforce authentication by using OptionalLogin mode. This is
		// needed for IP whitelisted bots: they have NO credentials to send.
		authCl, err = auth.NewAuthenticator(ctx, auth.OptionalLogin, c.parsedAuthOpts).Client()
		if err != nil {
			return
		}
	}

	userAgent := "isolated-go/" + IsolatedVersion
	if ver, err := version.GetStartupVersion(); err == nil && ver.InstanceID != "" {
		userAgent += fmt.Sprintf(" (%s@%s)", ver.PackageName, ver.InstanceID)
	}
	isolOpts := []isolatedclient.Option{
		isolatedclient.WithAuthClient(authCl),
		isolatedclient.WithUserAgent(userAgent),
	}
	if opts.AnonClient != nil {
		isolOpts = append(isolOpts, isolatedclient.WithAnonymousClient(opts.AnonClient))
	}
	isolCl = c.isolatedFlags.NewClient(isolOpts...)
	return
}

// CommandOptions is used to initialize an isolated command.
type CommandOptions struct {
	AuthClient      *http.Client
	AnonClient      *http.Client
	DefaultAuthOpts auth.Options
}
