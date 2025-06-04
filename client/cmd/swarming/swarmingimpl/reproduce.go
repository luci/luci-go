// Copyright 2021 The LUCI Authors.
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

//go:build !copybara
// +build !copybara

package swarmingimpl

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/base"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/output"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/lucictx"
	rdbcli "go.chromium.org/luci/resultdb/cli"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/swarming/client/swarming"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/runner"
)

// CmdReproduce returns an object for the `reproduce` subcommand.
func CmdReproduce(authFlags base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "reproduce -S <server> <task ID>",
		ShortDesc: "fetches the task command and runs it locally",
		LongDesc:  "Fetches a TaskRequest and runs the same commands that were run on the bot.",
		CommandRun: func() subcommands.CommandRun {
			return base.NewCommandRun(authFlags, &reproduceImpl{
				cipdDownloader:   downloadCIPDPackages,
				createInvocation: createInvocation,
			}, base.Features{
				MinArgs: 1,
				MaxArgs: 1,
				UsesCAS: true,
				OutputJSON: base.OutputJSON{
					Enabled: false,
				},
			})
		},
	}
}

type reproduceImpl struct {
	work        string
	out         string
	realm       string
	resultsHost string

	taskID string

	// cipdDownloader is used in testing to insert a mock CIPD downloader.
	cipdDownloader func(context.Context, string, map[string]ensure.PackageSlice) error
	// createInvocation is used in testing to insert a mock method.
	createInvocation func(context.Context, *http.Client, string, string) (lucictx.Exported, func(), error)
}

func (cmd *reproduceImpl) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(&cmd.work, "work", "work", "Directory to map the task input files into and execute the task. Will be cleared!")
	fs.StringVar(&cmd.out, "out", "out", "Directory that will hold the task results. Will be cleared!")
	fs.StringVar(&cmd.realm, "realm", "", "Realm to create invocation in if ResultDB is enabled.")
	fs.StringVar(&cmd.resultsHost, "results-host", chromeinfra.ResultDBHost, "Hostname of the ResultDB service to use. e.g. 'results.api.luci.app'.")
}

func (cmd *reproduceImpl) ParseInputs(ctx context.Context, args []string, env subcommands.Env, extra base.Extra) error {
	var err error
	if cmd.work, err = filepath.Abs(cmd.work); err != nil {
		return errors.Fmt("failed to get absolute representation of work directory: %w", err)
	}
	if cmd.out, err = filepath.Abs(cmd.out); err != nil {
		return errors.Fmt("failed to get absolute representation of out directory: %w", err)
	}
	cmd.taskID = args[0]
	return nil
}

func (cmd *reproduceImpl) Execute(ctx context.Context, svc swarming.Client, sink *output.Sink, extra base.Extra) error {
	tr, err := svc.TaskRequest(ctx, cmd.taskID)
	if err != nil {
		return errors.Fmt("failed to get task request: %s: %w", cmd.taskID, err)
	}

	// In practice, later slices are less likely to assume that there is a named
	// cache that is not available locally.
	properties := tr.TaskSlices[len(tr.TaskSlices)-1].Properties

	execCmd, err := cmd.prepareTaskRequestEnvironment(ctx, properties, svc, extra.AuthFlags)
	if err != nil {
		return errors.Fmt("failed to create command from task request: %w", err)
	}

	return cmd.executeTaskRequestCommand(ctx, tr, execCmd, extra.AuthFlags)
}

func (cmd *reproduceImpl) executeTaskRequestCommand(ctx context.Context, tr *swarmingv2.TaskRequestResponse, execCmd *exec.Cmd, auth base.AuthFlags) error {
	// Enable ResultDB if necessary.
	if tr.Resultdb != nil && tr.Resultdb.Enable {
		if cmd.realm == "" {
			return errors.New("must provide -realm if task request has ResultDB enabled")
		}
		authcli, err := auth.NewHTTPClient(ctx)
		if err != nil {
			return errors.Fmt("failed to create client: %w", err)
		}
		exported, invFinalizer, err := cmd.createInvocation(ctx, authcli, cmd.realm, cmd.resultsHost)
		if err != nil {
			return errors.Fmt("failed to create Invocation: %w", err)
		}
		defer invFinalizer()
		exported.SetInCmd(execCmd)
		defer func() { _ = exported.Close() }()
	}

	if err := execCmd.Start(); err != nil {
		return errors.Fmt("failed to start command: %v: %w", execCmd, err)
	}
	if err := execCmd.Wait(); err != nil {
		return errors.Fmt("failed to complete command: %v: %w", execCmd, err)
	}
	return nil
}

func (cmd *reproduceImpl) prepareTaskRequestEnvironment(ctx context.Context, properties *swarmingv2.TaskProperties, svc swarming.Client, auth base.AuthFlags) (*exec.Cmd, error) {
	execDir := cmd.work
	if properties.RelativeCwd != "" {
		// TODO(vadimsh): Forbid "..".
		execDir = filepath.Join(execDir, properties.RelativeCwd)
	}
	if err := prepareDir(execDir); err != nil {
		return nil, err
	}
	if err := prepareDir(cmd.out); err != nil {
		return nil, err
	}

	// Set environment variables.
	cmdEnvMap := environ.FromCtx(ctx)
	for _, env := range properties.Env {
		if env.Value == "" {
			cmdEnvMap.Remove(env.Key)
		} else {
			cmdEnvMap.Set(env.Key, env.Value)
		}
	}

	// Set environment prefixes.
	for _, prefix := range properties.EnvPrefixes {
		paths := make([]string, 0, len(prefix.Value)+1)
		for _, value := range prefix.Value {
			paths = append(paths, filepath.Clean(filepath.Join(cmd.work, value)))
		}
		cur, ok := cmdEnvMap.Lookup(prefix.Key)
		if ok {
			paths = append(paths, cur)
		}
		cmdEnvMap.Set(prefix.Key, strings.Join(paths, string(os.PathListSeparator)))
	}

	// Support RBE-CAS input in task request.
	if properties.CasInputRoot != nil {
		if _, err := svc.FilesFromCAS(ctx, cmd.work, properties.CasInputRoot); err != nil {
			return nil, errors.Fmt("failed to fetch files from RBE-CAS: %w", err)
		}
	}

	// Support CIPD package download in task request.
	if properties.CipdInput != nil {
		packages := properties.CipdInput.Packages
		slicesByPath := map[string]ensure.PackageSlice{}
		for _, pkg := range packages {
			path := pkg.Path
			// CIPD deals with 'root' as ''.
			if path == "." {
				path = ""
			}
			if _, ok := slicesByPath[path]; !ok {
				slicesByPath[path] = make(ensure.PackageSlice, 0, len(packages))
			}
			slicesByPath[path] = append(
				slicesByPath[path], ensure.PackageDef{UnresolvedVersion: pkg.Version, PackageTemplate: pkg.PackageName})
		}

		if err := cmd.cipdDownloader(ctx, cmd.work, slicesByPath); err != nil {
			return nil, err
		}
	}

	// Create a Command that can run the task request.
	processedCmds, err := runner.ProcessCommand(ctx, properties.Command, cmd.out, "")
	if err != nil {
		return nil, errors.Fmt("failed to process command in properties: %w", err)
	}

	execCmd := exec.CommandContext(ctx, processedCmds[0], processedCmds[1:]...)
	execCmd.Env = cmdEnvMap.Sorted()
	execCmd.Dir = execDir
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	return execCmd, nil
}

func downloadCIPDPackages(ctx context.Context, workdir string, slicesByPath map[string]ensure.PackageSlice) error {
	// Create CIPD client.
	client, err := cipd.NewClientFromEnv(ctx, cipd.ClientOptions{Root: workdir})
	if err != nil {
		return errors.Fmt("failed to create CIPD client: %w", err)
	}
	defer client.Close(ctx)

	// Resolve versions.
	resolver := cipd.Resolver{Client: client}
	resolved, err := resolver.Resolve(ctx, &ensure.File{
		ServiceURL:       client.Options().ServiceURL,
		PackagesBySubdir: slicesByPath,
	}, template.DefaultExpander())
	if err != nil {
		return errors.Fmt("failed to resolve CIPD package versions: %w", err)
	}

	// Download packages.
	if _, err := client.EnsurePackages(ctx, resolved.PackagesBySubdir, &cipd.EnsureOptions{
		Paranoia: resolved.ParanoidMode,
	}); err != nil {
		return errors.Fmt("failed to install or update CIPD packages: %w", err)
	}
	return nil

}

func prepareDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return errors.Fmt("failed to remove directory: %s: %w", dir, err)
	}
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return errors.Fmt("failed to create directory: %s: %w", dir, err)
	}
	return nil
}

func createInvocation(ctx context.Context, authcli *http.Client, realm string, resultsHost string) (lucictx.Exported, func(), error) {
	// TODO(vadimsh): Construct prpc.Client centrally in cmdbase with correct
	// user agent.
	recorder := resultpb.NewRecorderPRPCClient(&prpc.Client{
		C:       authcli,
		Host:    resultsHost,
		Options: prpc.DefaultOptions(),
	})

	invID, err := rdbcli.GenInvID(ctx)
	if err != nil {
		return nil, nil, err
	}
	md := metadata.MD{}
	invocation, err := recorder.CreateInvocation(ctx, &resultpb.CreateInvocationRequest{
		InvocationId: invID,
		Invocation: &resultpb.Invocation{
			Realm: realm,
		},
	}, grpc.Header(&md))
	if err != nil {
		return nil, nil, err
	}
	tks := md.Get("update-token")
	if len(tks) != 1 {
		return nil, nil, errors.New("Missing header: update-token")
	}
	exported, err := lucictx.Export(
		lucictx.SetResultDB(ctx, &lucictx.ResultDB{
			Hostname:          resultsHost,
			CurrentInvocation: &lucictx.ResultDBInvocation{Name: invocation.Name, UpdateToken: tks[0]},
		}))
	if err != nil {
		return nil, nil, errors.Fmt("failed to export context: %w", err)
	}

	return exported, func() {
		ctx = metadata.AppendToOutgoingContext(ctx, "update-token", tks[0])
		if _, err := recorder.FinalizeInvocation(ctx, &resultpb.FinalizeInvocationRequest{
			Name: invocation.Name,
		}); err != nil {
			logging.WithError(err).Warningf(ctx, "failed to finalize invocation")
		}
		fmt.Printf("created invocation = %s\n", rdbcli.MustReturnInvURL(resultsHost, invocation.Name))
	}, nil
}
