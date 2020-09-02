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
	"context"
	"flag"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cas"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/profiling"
)

var _ cli.ContextModificator = (*commonFlags)(nil)

type commonFlags struct {
	subcommands.CommandRunBase
	casFlags  cas.Flags
	logConfig logging.Config // for -log-level, used by ModifyContext
	profiler  profiling.Profiler
}

func (c *commonFlags) Init() {
	c.casFlags.Init(&c.Flags)

	c.logConfig.Level = logging.Warning
	c.logConfig.AddFlags(&c.Flags)
	c.profiler.AddFlags(&c.Flags)
}

func (c *commonFlags) Parse() error {
	// extract glog flag used in remote-apis-sdks
	logtostderr := flag.Lookup("logtostderr")
	if logtostderr == nil {
		return errors.Reason("logtostderr flag for glog not found").Err()
	}
	v := flag.Lookup("v")
	if v == nil {
		return errors.Reason("v flag for glog not found").Err()
	}

	if c.logConfig.Level == logging.Debug {
		logtostderr.Value.Set("true")
		v.Value.Set("9")
	}

	if err := c.profiler.Start(); err != nil {
		return err
	}

	return c.casFlags.Parse()
}

// ModifyContext implements cli.ContextModificator.
func (c *commonFlags) ModifyContext(ctx context.Context) context.Context {
	return c.logConfig.Set(ctx)
}
