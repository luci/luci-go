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

// +build !copybara

package lib

import (
	"context"
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
	clientswarming "go.chromium.org/luci/client/swarming"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/lucictx"
	rdbcli "go.chromium.org/luci/resultdb/cli"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
)

// CmdReproduce returns an object fo the `reproduce` subcommand.
func CmdReproduce(authFlags AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "reproduce -S <server> <task ID> ",
		ShortDesc: "reproduces a task locally",
		LongDesc:  "Fetches a TaskRequest and runs the same commands that were run on the bot.",
		CommandRun: func() subcommands.CommandRun {
			r := &reproduceRun{}
			r.init(authFlags)
			return r
		},
	}
}

type reproduceRun struct {
	commonFlags
	work string
	out  string
	// cipdDownloader is used in testing to insert a mock CIPD downloader.
	cipdDownloader func(context.Context, string, map[string]ensure.PackageSlice) error
	// createInvocation is used in testing to insert a mock method.
	createInvocation func(context.Context, *http.Client, string, string) (lucictx.Exported, func(), error)
	realm            string
	resultsHost      string
}

func (c *reproduceRun) init(authFlags AuthFlags) {
	c.commonFlags.Init(authFlags)

	c.Flags.StringVar(&c.work, "work", "work", "Directory to map the task input files into and execute the task.")
	c.Flags.StringVar(&c.out, "out", "out", "Directory that will hold the task results.")
	c.Flags.StringVar(&c.realm, "realm", "", "Realm to create invocation in if ResultDB is enabled.")
	c.Flags.StringVar(&c.resultsHost, "results-host", chromeinfra.ResultDBHost, "Hostname of the ResultDB service to usse. e.g. 'results.api.cr.dev'")
	c.cipdDownloader = downloadCIPDPackages
	c.createInvocation = createInvocation
}

func (c *reproduceRun) parse(args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 1 {
		return errors.Reason("must specify exactly one task id.").Err()
	}
	var err error
	if c.work, err = filepath.Abs(c.work); err != nil {
		return errors.Annotate(err, "failed to get absolute representation of work directory").Err()
	}
	if c.out, err = filepath.Abs(c.out); err != nil {
		return errors.Annotate(err, "failed to get absolute representation of out directory").Err()
	}
	return nil
}

func (c *reproduceRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if err := c.parse(args); err != nil {
		printError(a, err)
		return 1
	}
	if err := c.main(a, args, env); err != nil {
		printError(a, err)
		return 1
	}
	return 0
}

func (c *reproduceRun) main(a subcommands.Application, args []string, env subcommands.Env) error {
	ctx, cancel := context.WithCancel(c.defaultFlags.MakeLoggingContext(os.Stderr))
	defer cancel()
	defer signals.HandleInterrupt(cancel)()

	service, err := c.createSwarmingClient(ctx)
	if err != nil {
		return err
	}

	tr, err := service.GetTaskRequest(ctx, args[0])
	if err != nil {
		return errors.Annotate(err, "failed to get task request: %s", args[0]).Err()
	}
	// In practice, later slices are less likely to assume that there is a named cache
	// that is not available locally.
	properties := tr.TaskSlices[len(tr.TaskSlices)-1].Properties

	cmd, err := c.prepareTaskRequestEnvironment(ctx, properties, service)
	if err != nil {
		return errors.Annotate(err, "failed to create command from task request").Err()
	}

	return c.executeTaskRequestCommand(ctx, tr, cmd)
}

func (c *reproduceRun) executeTaskRequestCommand(ctx context.Context, tr *swarming.SwarmingRpcsTaskRequest, cmd *exec.Cmd) error {
	// Enable ResultDB if necessary.
	if tr.Resultdb != nil && tr.Resultdb.Enable {
		if c.realm == "" {
			return errors.Reason("must provide -realm if task request has ResultDB enabled").Err()
		}
		authcli, err := c.authFlags.NewHTTPClient(ctx)
		if err != nil {
			return errors.Annotate(err, "failed to create client").Err()
		}
		exported, invFinalizer, err := c.createInvocation(ctx, authcli, c.realm, c.resultsHost)
		if err != nil {
			return errors.Annotate(err, "failed to create Invocation").Err()
		}
		defer invFinalizer()
		exported.SetInCmd(cmd)
		defer exported.Close()
	}

	if err := cmd.Start(); err != nil {
		return errors.Annotate(err, "failed to start command: %v", cmd).Err()
	}
	if err := cmd.Wait(); err != nil {
		return errors.Annotate(err, "failed to complete command: %v", cmd).Err()
	}
	return nil
}

func (c *reproduceRun) prepareTaskRequestEnvironment(ctx context.Context, properties *swarming.SwarmingRpcsTaskProperties, service swarmingService) (*exec.Cmd, error) {
	execDir := c.work
	if properties.RelativeCwd != "" {
		execDir = filepath.Join(execDir, properties.RelativeCwd)
	}
	if err := prepareDir(execDir); err != nil {
		return nil, err
	}
	if err := prepareDir(c.out); err != nil {
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
			paths = append(paths, filepath.Clean(filepath.Join(c.work, value)))
		}
		cur, ok := cmdEnvMap.Get(prefix.Key)
		if ok {
			paths = append(paths, cur)
		}
		cmdEnvMap.Set(prefix.Key, strings.Join(paths, string(os.PathListSeparator)))
	}

	// Download input files.
	if properties.InputsRef != nil && properties.InputsRef.Isolated != "" && properties.CasInputRoot != nil {
		return nil, errors.Reason("fetched TaskRequest has files from Isolate and RBE-CAS").Err()
	}

	// Support isolated input in task request.
	if properties.InputsRef != nil && properties.InputsRef.Isolated != "" {
		if _, err := service.GetFilesFromIsolate(ctx, c.work, properties.InputsRef); err != nil {
			return nil, errors.Annotate(err, "failed to fetch files from isolate").Err()
		}
	}

	// Support RBE-CAS input in task request.
	if properties.CasInputRoot != nil {
		cascli, err := c.authFlags.NewCASClient(ctx, properties.CasInputRoot.CasInstance)
		if err != nil {
			return nil, errors.Annotate(err, "failed to fetch RBE-CAS client").Err()
		}
		if _, err := service.GetFilesFromCAS(ctx, c.work, cascli, properties.CasInputRoot); err != nil {
			return nil, errors.Annotate(err, "failed to fetched friles from RBE-CAS").Err()
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

		if err := c.cipdDownloader(ctx, c.work, slicesByPath); err != nil {
			return nil, err
		}
	}

	// Create a Comand that can run the task request.
	processedCmds, err := clientswarming.ProcessCommand(ctx, properties.Command, c.out, "")
	if err != nil {
		return nil, errors.Annotate(err, "failed to process command in properties").Err()
	}

	cmd := exec.CommandContext(ctx, processedCmds[0], processedCmds[1:]...)
	cmd.Env = cmdEnvMap.Sorted()
	cmd.Dir = execDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd, nil
}

func downloadCIPDPackages(ctx context.Context, workdir string, slicesByPath map[string]ensure.PackageSlice) error {
	// Create CIPD client.
	opts := cipd.ClientOptions{
		Root:       workdir,
		ServiceURL: chromeinfra.CIPDServiceURL,
	}
	if err := opts.LoadFromEnv(cli.MakeGetEnv(ctx)); err != nil {
		return errors.Annotate(err, "failed to create CIPD client").Err()
	}
	client, err := cipd.NewClient(opts)
	if err != nil {
		return errors.Annotate(err, "failed to create CIPD client").Err()
	}
	defer client.Close(ctx)

	// Resolve versions.
	resolver := cipd.Resolver{Client: client}
	resolved, err := resolver.Resolve(
		ctx, &ensure.File{ServiceURL: chromeinfra.CIPDServiceURL, PackagesBySubdir: slicesByPath}, template.DefaultExpander())
	if err != nil {
		return errors.Annotate(err, "failed to resolve CIPD package versions").Err()
	}

	// Download packages.
	if _, err := client.EnsurePackages(ctx, resolved.PackagesBySubdir, resolved.ParanoidMode, false); err != nil {
		return errors.Annotate(err, "failed to install or update CIPD packages").Err()
	}
	return nil

}

func prepareDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return errors.Annotate(err, "failed to remove directory: %s", dir).Err()
	}
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return errors.Annotate(err, "failed to create directory: %s", dir).Err()
	}
	return nil
}

func createInvocation(ctx context.Context, authcli *http.Client, realm string, resultsHost string) (lucictx.Exported, func(), error) {
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
		return nil, nil, errors.Reason("Missing header: update-token").Err()
	}
	exported, err := lucictx.Export(
		lucictx.SetResultDB(ctx, &lucictx.ResultDB{
			Hostname:          resultsHost,
			CurrentInvocation: &lucictx.ResultDBInvocation{Name: invocation.Name, UpdateToken: tks[0]},
		}))
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to export context").Err()
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
