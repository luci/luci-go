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

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/client/butler/output"
	logdogOut "go.chromium.org/luci/logdog/client/butler/output/logdog"
	"go.chromium.org/luci/logdog/common/types"
)

var testingOutputCtxKey = "holds logdog Output in test mode"

func mkLogdogOutput(ctx context.Context, opts *bbpb.BuildInfra_LogDog) (output.Output, error) {
	if opts.Hostname == "test.example.com" {
		if out, ok := ctx.Value(&testingOutputCtxKey).(output.Output); ok {
			return out, nil
		}
		return nil, errors.New("opts.Hostname == test.example.com but no Output in context")
	}
	logging.Infof(ctx, "Register Logdog prefix %q in project %q", opts.Prefix, opts.Project)
	auth, err := logdogOut.RealmsAwareAuth(ctx)
	if err != nil {
		return nil, err
	}
	return (&logdogOut.Config{
		Auth:    auth,
		Host:    opts.Hostname,
		Project: opts.Project,
		Prefix:  types.StreamName(opts.Prefix),
	}).Register(ctx)
}
