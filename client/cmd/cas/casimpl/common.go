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

package casimpl

import (
	"context"
	"flag"
	"fmt"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/profiling"
)

// Version is reported by "version" subcommand and put into logs.
const Version = "cas 0.2.0"

// AuthFlags is an interface to register auth flags and create RBE Client.
type AuthFlags interface {
	// Register registers auth flags to the given flag set. e.g. -service-account-json.
	Register(f *flag.FlagSet)

	// Parse parses auth flags.
	Parse() error

	// NewRBEClient creates an authorised RBE Client.
	NewRBEClient(ctx context.Context, addr string, instance string, readOnly bool) (*client.Client, error)
}

var _ cli.ContextModificator = (*commonFlags)(nil)

type commonFlags struct {
	subcommands.CommandRunBase
	casFlags  casclient.Flags
	logConfig logging.Config // for -log-level, used by ModifyContext
	profiler  profiling.Profiler
	authFlags AuthFlags
}

func (c *commonFlags) Init(authFlags AuthFlags) {
	c.casFlags.Init(&c.Flags)
	c.authFlags = authFlags
	c.authFlags.Register(&c.Flags)
	c.logConfig.Level = logging.Warning
	c.logConfig.AddFlags(&c.Flags)
	c.profiler.AddFlags(&c.Flags)
}

func (c *commonFlags) Parse() error {
	verbosity := 0
	switch c.logConfig.Level {
	case logging.Debug:
		verbosity = 9
	case logging.Info:
		verbosity = 2
	}
	if verbosity != 0 {
		if err := enableGlogVerbosity(verbosity); err != nil {
			return err
		}
	}

	if err := c.profiler.Start(); err != nil {
		return err
	}
	if err := c.authFlags.Parse(); err != nil {
		return err
	}

	return c.casFlags.Parse()
}

// ModifyContext implements cli.ContextModificator.
func (c *commonFlags) ModifyContext(ctx context.Context) context.Context {
	return c.logConfig.Set(ctx)
}

func enableGlogVerbosity(level int) error {
	// extract glog flag used in remote-apis-sdks, if available
	logtostderr := flag.Lookup("logtostderr")
	if logtostderr == nil {
		return nil
	}
	if err := logtostderr.Value.Set("true"); err != nil {
		return errors.Annotate(err, "failed to set logstderr to true").Err()
	}
	v := flag.Lookup("v")
	if v == nil {
		return errors.Reason("v flag for glog not found").Err()
	}
	if err := v.Value.Set(fmt.Sprintf("%d", level)); err != nil {
		return errors.Annotate(err, "failed to set verbosity level").Err()
	}
	return nil
}
