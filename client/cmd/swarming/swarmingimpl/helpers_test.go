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

package swarmingimpl

import (
	"bytes"
	"context"
	"flag"
	"net/http"

	rbeclient "github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/base"
	"go.chromium.org/luci/swarming/client/swarming"
	"go.chromium.org/luci/swarming/client/swarming/swarmingtest"
)

var _ base.AuthFlags = (*testAuthFlags)(nil)

type testAuthFlags struct{}

func (af *testAuthFlags) Register(_ *flag.FlagSet) {}

func (af *testAuthFlags) Parse() error { return nil }

func (af *testAuthFlags) NewHTTPClient(_ context.Context) (*http.Client, error) {
	return nil, nil
}

func (af *testAuthFlags) NewRBEClient(_ context.Context, _ string, _ string) (*rbeclient.Client, error) {
	return nil, nil
}

// SubcommandTest runs the subcommand in a test mode, mocking dependencies.
//
// Returns an error, exit code, and captured stdout and stderr.
func SubcommandTest(ctx context.Context, cmd func(base.AuthFlags) *subcommands.Command, args []string, env subcommands.Env, svc swarming.Client) (err error, code int, stdout, stderr string) {
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	code = subcommands.Run(&subcommands.DefaultApplication{
		Name: "testing-app",
		Commands: []*subcommands.Command{
			{
				UsageLine: "testing-cmd",
				CommandRun: func() subcommands.CommandRun {
					if svc == nil {
						svc = &swarmingtest.Client{}
					}
					cr := cmd(&testAuthFlags{}).CommandRun().(*base.CommandRun)
					cr.TestingMocks(ctx, svc, env, &err, &stdoutBuf, &stderrBuf)
					return cr
				},
			},
		},
	}, append([]string{"testing-cmd"}, args...))

	return err, code, stdoutBuf.String(), stderrBuf.String()
}
