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

package host

import (
	"context"
	"os"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/logdog/client/butler"
	"go.chromium.org/luci/logdog/client/butler/streamserver"
)

// overridden in tests
var bufferLogs = true

// startButler sets up a Butler streamserver, and exports it to the environment.
func startButler(ctx context.Context, opts *Options) (*butler.Butler, error) {
	butlerCtx := ctx
	if logging.GetLevel(ctx) > opts.ButlerLogLevel {
		butlerCtx = logging.SetLevel(ctx, opts.ButlerLogLevel)
	}

	butler, err := butler.New(butlerCtx, butler.Config{
		BufferLogs: bufferLogs,
		GlobalTags: opts.logdogTags,
		Output:     opts.LogdogOutput,
	})
	if err != nil {
		return nil, err
	}

	streamServerCtx := ctx
	if !opts.StreamServerDisableLogAdjustment {
		streamServerCtx = logging.SetLevel(ctx, logging.Warning)
	}
	sserv, err := streamserver.New(streamServerCtx, opts.streamServerPath)
	if err != nil {
		return nil, err
	}
	if err := sserv.Listen(); err != nil {
		return nil, err
	}

	butler.AddStreamServer(sserv)

	toExport := opts.LogdogOutput.URLConstructionEnv()
	toExport.StreamServerURI = sserv.Address()
	env := environ.New(nil)
	toExport.Augment(env)

	return butler, env.Iter(os.Setenv)
}
