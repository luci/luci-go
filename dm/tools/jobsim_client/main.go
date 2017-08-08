// Copyright 2016 The LUCI Authors.
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

// Jobsim client is a self-contained binary that implements various toy job
// algorithms for use in testing DM with live distributors (like swarming).
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/retry"
	dm "go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/lucictx"
)

type cmdRun struct {
	subcommands.CommandRunBase

	context.Context

	exAuth *dm.Execution_Auth

	questDescPath string
	questDesc     *dm.Quest_Desc

	dmHost string

	client dm.DepsClient

	cmd *subcommands.Command
}

func (r *cmdRun) registerOptions() {
	r.Flags.StringVar(&r.questDescPath, "quest-desc-path", "",
		"The path to a JSONPB encoded dm.Quest.Data.Desc message of how this client"+
			" was invoked")
	r.Flags.StringVar(&r.dmHost, "dm-host", "", "The hostname of the DM service")
}

func loadJSONPB(c context.Context, flavor, path string, msg proto.Message) {
	if path == "" {
		logging.Errorf(c, "processing %q: required parameter", flavor)
		os.Exit(-1)
	}

	fil, err := os.Open(path)
	if err != nil {
		logging.Errorf(c, "processing %q: %s", flavor, err)
		os.Exit(-1)
	}
	defer fil.Close()
	if err := jsonpb.Unmarshal(fil, msg); err != nil {
		panic(err)
	}
}

func (r *cmdRun) start(a subcommands.Application, cr subcommands.CommandRun, env subcommands.Env, c *subcommands.Command) {
	r.cmd = c

	r.Context = cli.GetContext(a, cr, env)

	r.exAuth = &dm.Execution_Auth{}
	s := lucictx.GetSwarming(r.Context)
	if s == nil || s.SecretBytes == nil {
		panic("LUCI_CONTEXT['swarming']['secret_bytes'] is missing")
	}
	if err := jsonpb.Unmarshal(bytes.NewReader(s.SecretBytes), r.exAuth); err != nil {
		panic(fmt.Errorf("while decoding execution auth: %s", err))
	}

	r.questDesc = &dm.Quest_Desc{}
	loadJSONPB(r, "quest-desc-path", r.questDescPath, r.questDesc)

	// initialize rpc client
	r.client = dm.NewDepsPRPCClient(&prpc.Client{
		Host: r.dmHost,
		Options: &prpc.Options{
			Retry: retry.Default,
		},
	})

	key := make([]byte, 32)
	_, err := cryptorand.Read(r, key)
	if err != nil {
		panic(err)
	}

	req := &dm.ActivateExecutionReq{
		Auth:           r.exAuth,
		ExecutionToken: key,
	}
	err = retry.Retry(r, retry.Default, func() error {
		_, err := r.client.ActivateExecution(r, req)
		return err
	}, retry.LogCallback(r, "ActivateExecution"))
	if err != nil {
		panic(err)
	}

	// activated with key!
	r.exAuth.Token = key
}

func (r *cmdRun) depOn(jobProperties ...interface{}) (data []string, stop bool) {
	req := &dm.EnsureGraphDataReq{
		ForExecution: r.exAuth,

		Quest:        make([]*dm.Quest_Desc, len(jobProperties)),
		QuestAttempt: make([]*dm.AttemptList_Nums, len(jobProperties)),

		Include: &dm.EnsureGraphDataReq_Include{
			Attempt: &dm.EnsureGraphDataReq_Include_Options{Result: true},
		},
	}

	for i, jProps := range jobProperties {
		data, err := json.Marshal(jProps)
		if err != nil {
			panic(err)
		}
		req.Quest[i] = proto.Clone(r.questDesc).(*dm.Quest_Desc)
		req.Quest[i].Parameters = string(data)
		req.QuestAttempt[i] = &dm.AttemptList_Nums{Nums: []uint32{1}}
	}

	rsp := (*dm.EnsureGraphDataRsp)(nil)
	err := retry.Retry(r, retry.Default, func() (err error) {
		rsp, err = r.client.EnsureGraphData(r, req)
		return
	}, retry.LogCallback(r, "EnsureGraphData"))
	if err != nil {
		panic(err)
	}
	if rsp.ShouldHalt {
		logging.Infof(r, "got ShouldHalt")
		return nil, true
	}

	ret := make([]string, len(jobProperties))
	for i, qid := range rsp.QuestIds {
		ret[i] = rsp.Result.Quests[qid.Id].Attempts[1].Data.GetFinished().Data.Object
	}

	return ret, false
}

func (r *cmdRun) finish(data interface{}) int {
	encData, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	err = retry.Retry(r, retry.Default, func() error {
		_, err := r.client.FinishAttempt(r, &dm.FinishAttemptReq{
			Auth: r.exAuth, Data: dm.NewJsonResult(string(encData)),
		})
		return err
	}, retry.LogCallback(r, "FinishAttempt"))
	if err != nil {
		panic(err)
	}
	return 0
}

// argErr prints an err and usage to stderr and returns an exit code.
func (r *cmdRun) argErr(format string, a ...interface{}) int {
	if format != "" {
		fmt.Fprintf(os.Stderr, format+"\n", a...)
	}
	fmt.Fprintln(os.Stderr, r.cmd.ShortDesc)
	fmt.Fprintln(os.Stderr, r.cmd.UsageLine)
	fmt.Fprintln(os.Stderr, "\nFlags:")
	r.Flags.PrintDefaults()
	return -1
}

var application = &cli.Application{
	Name:  "jobsim_client",
	Title: "Executable Job simulator client",
	Context: func(ctx context.Context) context.Context {
		return gologger.StdConfig.Use(ctx)
	},
	Commands: []*subcommands.Command{
		cmdEditDistance,
		subcommands.CmdHelp,
	},
}

func main() {
	os.Exit(subcommands.Run(application, os.Args[1:]))
}
