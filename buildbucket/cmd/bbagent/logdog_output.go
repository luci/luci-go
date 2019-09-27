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

package main

import (
	"context"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/logging"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/logdog/client/butler/output"
	"go.chromium.org/luci/logdog/client/butler/output/logdog"
	"go.chromium.org/luci/logdog/common/types"
)

func mkLogdogOutput(ctx context.Context, opts *bbpb.BuildInfra_LogDog) (output.Output, error) {
	// butler debug logging is WAY too verbose.
	ctx = logging.SetLevel(ctx, logging.Info)

	return (&logdog.Config{
		Auth: auth.NewAuthenticator(ctx, auth.SilentLogin, auth.Options{
			Scopes: []string{
				auth.OAuthScopeEmail,
				"https://www.googleapis.com/auth/cloud-platform",
			},
			MonitorAs: "bbagent/logdog",
		}),
		Host:    opts.Hostname,
		Project: opts.Project,
		Prefix:  types.StreamName(opts.Prefix),
	}).Register(ctx)
}
