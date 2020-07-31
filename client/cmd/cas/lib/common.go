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

package lib

import (
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/client/cas"
	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/common/runtime/profiling"
)

type commonFlags struct {
	subcommands.CommandRunBase
	casFlags       cas.Flags
	defaultFlags   common.Flags
	authFlags      authcli.Flags
	parsedAuthOpts auth.Options
	profilerFlags  profiling.Profiler
}

func (c *commonFlags) Init(authOpts auth.Options) {
	c.defaultFlags.Init(&c.Flags)
	c.authFlags.Register(&c.Flags, authOpts)
	c.profilerFlags.AddFlags(&c.Flags)
}

func (c *commonFlags) Parse() error {
	var err error
	if err = c.defaultFlags.Parse(); err != nil {
		return err
	}

	if err = c.casFlags.Parse(); err != nil {
		return err
	}

	if err := c.profilerFlags.Start(); err != nil {
		return err
	}
	c.parsedAuthOpts, err = c.authFlags.Options()
	return err
}
