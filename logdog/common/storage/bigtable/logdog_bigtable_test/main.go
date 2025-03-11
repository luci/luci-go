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

// Package main implements a simple CLI tool to load and interact with storage
// data in Google BigTable data.
package main

import (
	"context"
	"flag"
	"io"
	"os"

	cloudBT "cloud.google.com/go/bigtable"
	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"
	"google.golang.org/api/option"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"

	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/storage/bigtable"
	"go.chromium.org/luci/logdog/common/storage/memory"
	"go.chromium.org/luci/logdog/common/types"
)

////////////////////////////////////////////////////////////////////////////////
// main
////////////////////////////////////////////////////////////////////////////////

type application struct {
	cli.Application

	authOpts auth.Options

	btProject  string
	btInstance string
	btLogTable string
}

func getApplication(base subcommands.Application) (*application, context.Context) {
	app := base.(*application)
	return app, app.Context(context.Background())
}

func (app *application) addFlags(fs *flag.FlagSet) {
	fs.StringVar(&app.btProject, "bt-project", "", "BigTable project name (required)")
	fs.StringVar(&app.btInstance, "bt-instance", "", "BigTable instance name (required)")
	fs.StringVar(&app.btLogTable, "bt-log-table", "", "BigTable log table name (required)")
}

func (app *application) getBigTableClient(c context.Context) (*cloudBT.Client, error) {
	a := auth.NewAuthenticator(c, auth.SilentLogin, app.authOpts)
	tsrc, err := a.TokenSource()
	if err != nil {
		return nil, errors.Annotate(err, "failed to get token source").Err()
	}

	client, err := cloudBT.NewClient(c, app.btProject, app.btInstance, option.WithTokenSource(tsrc))
	if err != nil {
		return nil, errors.Annotate(err, "failed to get BigTable client").Err()
	}

	return client, nil
}

func (app *application) getStorage(btc *cloudBT.Client) storage.Storage {
	return &bigtable.Storage{
		Client: btc,

		LogTable: app.btLogTable,
		Cache:    &memory.Cache{},
	}
}

func mainImpl(c context.Context, defaultAuthOpts auth.Options, args []string) int {
	c = gologger.StdConfig.Use(c)

	logConfig := log.Config{
		Level: log.Warning,
	}

	defaultAuthOpts.Scopes = append([]string{auth.OAuthScopeEmail}, bigtable.StorageScopes...)
	var authFlags authcli.Flags

	app := application{
		Application: cli.Application{
			Name:  "BigTable Storage Utility",
			Title: "BigTable Storage Utility",
			Context: func(c context.Context) context.Context {
				// Install configured logger.
				c = logConfig.Set(gologger.StdConfig.Use(c))
				return c
			},

			Commands: []*subcommands.Command{
				subcommands.CmdHelp,

				&subcommandGet,
				&subcommandTail,

				authcli.SubcommandLogin(defaultAuthOpts, "auth-login", false),
				authcli.SubcommandLogout(defaultAuthOpts, "auth-logout", false),
				authcli.SubcommandInfo(defaultAuthOpts, "auth-info", false),
			},
		},
	}

	fs := flag.NewFlagSet("flags", flag.ExitOnError)
	app.addFlags(fs)
	logConfig.AddFlags(fs)
	authFlags.Register(fs, defaultAuthOpts)
	fs.Parse(args)

	switch {
	case app.btProject == "":
		log.Errorf(c, "Missing required argument (-bt-project).")
		return 1

	case app.btInstance == "":
		log.Errorf(c, "Missing required argument (-bt-instance).")
		return 1

	case app.btLogTable == "":
		log.Errorf(c, "Missing required argument (-bt-log-table).")
		return 1
	}

	// Process authentication options.
	var err error
	app.authOpts, err = authFlags.Options()
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create auth options.")
		return 1
	}

	// Execute our subcommand.
	return subcommands.Run(&app, fs.Args())
}

func main() {
	os.Exit(mainImpl(context.Background(), chromeinfra.DefaultAuthOptions(), os.Args[1:]))
}

func renderErr(c context.Context, err error) {
	log.Errorf(c, "Error encountered during operation: %s\n%s", err,
		errors.RenderStack(err))
}

func unmarshalAndDump(c context.Context, out io.Writer, data []byte, msg proto.Message) error {
	if data != nil {
		if err := proto.Unmarshal(data, msg); err != nil {
			log.WithError(err).Errorf(c, "Failed to unmarshal protobuf.")
			return err
		}
	}

	if err := proto.MarshalText(out, msg); err != nil {
		log.WithError(err).Errorf(c, "Failed to dump protobuf to output.")
		return err
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// Subcommand: get
////////////////////////////////////////////////////////////////////////////////

type cmdRunGet struct {
	subcommands.CommandRunBase

	project string
	path    string

	index  int
	limit  int
	rounds int
}

var subcommandGet = subcommands.Command{
	UsageLine: "get",
	ShortDesc: "Performs a Storage Get operation.",
	CommandRun: func() subcommands.CommandRun {
		var cmd cmdRunGet

		cmd.Flags.StringVar(&cmd.project, "project", "", "Log stream project name.")
		cmd.Flags.StringVar(&cmd.path, "path", "", "Log stream path.")
		cmd.Flags.IntVar(&cmd.index, "index", 0, "The index to fetch.")
		cmd.Flags.IntVar(&cmd.limit, "limit", 0, "The log entry limit.")
		cmd.Flags.IntVar(&cmd.rounds, "rounds", 1, "Number of rounds to run.")

		return &cmd
	},
}

func (cmd *cmdRunGet) Run(baseApp subcommands.Application, args []string, _ subcommands.Env) int {
	app, c := getApplication(baseApp)

	switch {
	case cmd.project == "":
		log.Errorf(c, "Missing required argument (-project).")
		return 1
	case cmd.path == "":
		log.Errorf(c, "Missing required argument (-path).")
		return 1
	}

	btClient, err := app.getBigTableClient(c)
	if err != nil {
		renderErr(c, errors.Annotate(err, "failed to create storage client").Err())
		return 1
	}
	defer btClient.Close()

	stClient := app.getStorage(btClient)

	for round := 0; round < cmd.rounds; round++ {
		log.Infof(c, "Get round %d.", round+1)

		var innerErr error
		err = stClient.Get(c, storage.GetRequest{
			Project: cmd.project,
			Path:    types.StreamPath(cmd.path),
			Index:   types.MessageIndex(cmd.index),
			Limit:   cmd.limit,
		}, func(e *storage.Entry) bool {
			le, err := e.GetLogEntry()
			if err != nil {
				log.WithError(err).Errorf(c, "Failed to unmarshal log entry.")
				return false
			}

			log.Fields{
				"index": le.StreamIndex,
			}.Infof(c, "Fetched log entry.")

			if innerErr = unmarshalAndDump(c, os.Stdout, nil, le); innerErr != nil {
				return false
			}
			return true
		})
		switch {
		case innerErr != nil:
			renderErr(c, errors.Annotate(err, "failed to process fetched log entries").Err())
			return 1

		case err != nil:
			renderErr(c, errors.Annotate(err, "Failed to Get log entries.").Err())
			return 1
		}
	}

	return 0
}

////////////////////////////////////////////////////////////////////////////////
// Subcommand: tail
////////////////////////////////////////////////////////////////////////////////

type cmdRunTail struct {
	subcommands.CommandRunBase

	project string
	path    string
	rounds  int
}

var subcommandTail = subcommands.Command{
	UsageLine: "tail",
	ShortDesc: "Performs a Storage Tail operation.",
	CommandRun: func() subcommands.CommandRun {
		var cmd cmdRunTail

		cmd.Flags.StringVar(&cmd.project, "project", "", "Log stream project name.")
		cmd.Flags.StringVar(&cmd.path, "path", "", "Log stream path.")
		cmd.Flags.IntVar(&cmd.rounds, "rounds", 1, "Number of rounds to run.")

		return &cmd
	},
}

func (cmd *cmdRunTail) Run(baseApp subcommands.Application, args []string, _ subcommands.Env) int {
	app, c := getApplication(baseApp)

	switch {
	case cmd.project == "":
		log.Errorf(c, "Missing required argument (-project).")
		return 1
	case cmd.path == "":
		log.Errorf(c, "Missing required argument (-path).")
		return 1
	}

	btClient, err := app.getBigTableClient(c)
	if err != nil {
		renderErr(c, errors.Annotate(err, "failed to create storage client").Err())
		return 1
	}
	defer btClient.Close()

	stClient := app.getStorage(btClient)

	for round := 0; round < cmd.rounds; round++ {
		log.Infof(c, "Tail round %d.", round+1)
		e, err := stClient.Tail(c, cmd.project, types.StreamPath(cmd.path))
		if err != nil {
			renderErr(c, errors.Annotate(err, "failed to tail log entries").Err())
			return 1
		}

		if e == nil {
			log.Infof(c, "No log data to tail.")
			continue
		}

		le, err := e.GetLogEntry()
		if err != nil {
			renderErr(c, errors.Annotate(err, "failed to unmarshal log entry").Err())
			return 1
		}

		log.Fields{
			"index": le.StreamIndex,
			"size":  len(e.D),
		}.Debugf(c, "Dumping tail entry.")
		if err := unmarshalAndDump(c, os.Stdout, nil, le); err != nil {
			renderErr(c, errors.Annotate(err, "failed to dump log entry").Err())
			return 1
		}
	}

	return 0
}
