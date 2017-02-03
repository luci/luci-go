// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package main implements a simple CLI tool to load and interact with Google
// Storage archived data.
package main

import (
	"bytes"
	"flag"
	"io"
	"io/ioutil"
	"math"
	"os"

	"github.com/luci/luci-go/client/authcli"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/data/rand/mathrand"
	"github.com/luci/luci-go/common/data/recordio"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/gcloud/gs"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/logdog/api/logpb"
	"github.com/luci/luci-go/logdog/common/storage"
	"github.com/luci/luci-go/logdog/common/storage/archive"
	"github.com/luci/luci-go/logdog/common/types"

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/hardcoded/chromeinfra"
)

////////////////////////////////////////////////////////////////////////////////
// main
////////////////////////////////////////////////////////////////////////////////

type application struct {
	cli.Application

	authOpts auth.Options
}

func getApplication(base subcommands.Application) (*application, context.Context) {
	app := base.(*application)
	return app, app.Context(context.Background())
}

func (a *application) getGSClient(c context.Context) (gs.Client, error) {
	authenticator := auth.NewAuthenticator(c, auth.OptionalLogin, a.authOpts)
	transport, err := authenticator.Transport()
	if err != nil {
		return nil, errors.Annotate(err).Reason("failed to get auth transport").Err()
	}
	return gs.NewProdClient(c, transport)
}

func mainImpl(c context.Context, defaultAuthOpts auth.Options, args []string) int {
	c = gologger.StdConfig.Use(c)

	logConfig := log.Config{
		Level: log.Warning,
	}

	defaultAuthOpts.Scopes = append([]string{auth.OAuthScopeEmail}, gs.ReadOnlyScopes...)
	var authFlags authcli.Flags

	a := application{
		Application: cli.Application{
			Name:  "Archive Storage Utility",
			Title: "Archive Storage Utility",
			Context: func(c context.Context) context.Context {
				// Install configured logger.
				c = logConfig.Set(gologger.StdConfig.Use(c))
				return c
			},

			Commands: []*subcommands.Command{
				subcommands.CmdHelp,

				&subcommandDumpIndex,
				&subcommandDumpStream,
				&subcommandGet,
				&subcommandTail,

				authcli.SubcommandLogin(defaultAuthOpts, "auth-login", false),
				authcli.SubcommandLogout(defaultAuthOpts, "auth-logout", false),
				authcli.SubcommandInfo(defaultAuthOpts, "auth-info", false),
			},
		},
	}

	fs := flag.NewFlagSet("flags", flag.ExitOnError)
	logConfig.AddFlags(fs)
	authFlags.Register(fs, defaultAuthOpts)
	fs.Parse(args)

	// Process authentication options.
	var err error
	a.authOpts, err = authFlags.Options()
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create auth options.")
		return 1
	}

	// Execute our subcommand.
	return subcommands.Run(&a, fs.Args())
}

func main() {
	mathrand.SeedRandomly()
	os.Exit(mainImpl(context.Background(), chromeinfra.DefaultAuthOptions(), os.Args[1:]))
}

func renderErr(c context.Context, err error) {
	rerr := errors.RenderStack(err)

	var buf bytes.Buffer
	rerr.DumpTo(&buf)
	log.Errorf(c, "Error encountered during operation: %s\n%s", err, buf.Bytes())
}

////////////////////////////////////////////////////////////////////////////////
// Subcommand: dump-index
////////////////////////////////////////////////////////////////////////////////

type cmdRunDumpIndex struct {
	subcommands.CommandRunBase

	path string
}

var subcommandDumpIndex = subcommands.Command{
	UsageLine: "dump-index",
	ShortDesc: "Dumps the contents of an index protobuf.",
	CommandRun: func() subcommands.CommandRun {
		var cmd cmdRunDumpIndex

		cmd.Flags.StringVar(&cmd.path, "path", "", "Google Storage path to the index protobuf.")

		return &cmd
	},
}

func (cmd *cmdRunDumpIndex) Run(baseApp subcommands.Application, args []string, _ subcommands.Env) int {
	app, c := getApplication(baseApp)

	if cmd.path == "" {
		log.Errorf(c, "Missing required argument (-path).")
		return 1
	}
	path := gs.Path(cmd.path)

	client, err := app.getGSClient(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create GS client.")
		return 1
	}
	defer client.Close()

	reader, err := client.NewReader(path, 0, -1)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create GS reader.")
		return 1
	}
	defer reader.Close()

	data, err := ioutil.ReadAll(reader)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to read index data from GS.")
		return 1
	}
	log.Debugf(c, "Loaded %d byte(s).", len(data))

	var index logpb.LogIndex
	if err := unmarshalAndDump(c, os.Stdout, data, &index); err != nil {
		log.WithError(err).Errorf(c, "Failed to dump index protobuf.")
		return 1
	}
	return 0
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
// Subcommand: dump-stream
////////////////////////////////////////////////////////////////////////////////

type cmdRunDumpStream struct {
	subcommands.CommandRunBase

	path string
}

var subcommandDumpStream = subcommands.Command{
	UsageLine: "dump-stream",
	ShortDesc: "Dumps the contents of a log stream protobuf.",
	CommandRun: func() subcommands.CommandRun {
		var cmd cmdRunDumpStream

		cmd.Flags.StringVar(&cmd.path, "path", "", "Google Storage path to the stream protobuf.")

		return &cmd
	},
}

func (cmd *cmdRunDumpStream) Run(baseApp subcommands.Application, args []string, _ subcommands.Env) int {
	app, c := getApplication(baseApp)

	if cmd.path == "" {
		log.Errorf(c, "Missing required argument (-path).")
		return 1
	}
	path := gs.Path(cmd.path)

	client, err := app.getGSClient(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create GS client.")
		return 1
	}
	defer client.Close()

	reader, err := client.NewReader(path, 0, -1)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create GS reader.")
		return 1
	}
	defer reader.Close()

	descFrame := true
	err = dumpRecordIO(c, reader, func(c context.Context, d []byte) error {
		if descFrame {
			descFrame = false

			var desc logpb.LogStreamDescriptor
			if err := unmarshalAndDump(c, os.Stdout, d, &desc); err != nil {
				return errors.Annotate(err).Reason("failed to dump log descriptor").Err()
			}
			return nil
		}

		var entry logpb.LogEntry
		if err := unmarshalAndDump(c, os.Stdout, d, &entry); err != nil {
			return errors.Annotate(err).Reason("failed to dump log entry").Err()
		}
		return nil
	})
	if err != nil {
		renderErr(c, err)
		return 1
	}

	return 0
}

////////////////////////////////////////////////////////////////////////////////
// Subcommand: get
////////////////////////////////////////////////////////////////////////////////

type cmdRunGet struct {
	subcommands.CommandRunBase

	indexPath  string
	streamPath string

	index int
	limit int
}

var subcommandGet = subcommands.Command{
	UsageLine: "get",
	ShortDesc: "Performs a Storage Get operation.",
	CommandRun: func() subcommands.CommandRun {
		var cmd cmdRunGet

		cmd.Flags.StringVar(&cmd.indexPath, "index-path", "", "Google Storage path to the index protobuf.")
		cmd.Flags.StringVar(&cmd.streamPath, "stream-path", "", "Google Storage path to the stream protobuf.")
		cmd.Flags.IntVar(&cmd.index, "index", 0, "The index to fetch.")
		cmd.Flags.IntVar(&cmd.limit, "limit", 0, "The log entry limit.")

		return &cmd
	},
}

func (cmd *cmdRunGet) Run(baseApp subcommands.Application, args []string, _ subcommands.Env) int {
	app, c := getApplication(baseApp)

	switch {
	case cmd.indexPath == "":
		log.Errorf(c, "Missing required argument (-index-path).")
		return 1
	case cmd.streamPath == "":
		log.Errorf(c, "Missing required argument (-stream-path).")
		return 1
	}

	client, err := app.getGSClient(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create GS client.")
		return 1
	}
	defer client.Close()

	stClient, err := archive.New(c, archive.Options{
		Index:  gs.Path(cmd.indexPath),
		Stream: gs.Path(cmd.streamPath),
		Client: client,
	})
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create storage client.")
		return 1
	}
	defer stClient.Close()

	var innerErr error
	err = stClient.Get(storage.GetRequest{
		Index: types.MessageIndex(cmd.index),
		Limit: cmd.limit,
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
		log.WithError(innerErr).Errorf(c, "Failed to process fetched log entries.")
		return 1

	case err != nil:
		log.WithError(err).Errorf(c, "Failed to Get log entries.")
		return 1

	default:
		return 0
	}
}

////////////////////////////////////////////////////////////////////////////////
// Subcommand: tail
////////////////////////////////////////////////////////////////////////////////

type cmdRunTail struct {
	subcommands.CommandRunBase

	indexPath  string
	streamPath string
}

var subcommandTail = subcommands.Command{
	UsageLine: "tail",
	ShortDesc: "Performs a Storage Tail operation.",
	CommandRun: func() subcommands.CommandRun {
		var cmd cmdRunTail

		cmd.Flags.StringVar(&cmd.indexPath, "index-path", "", "Google Storage path to the index protobuf.")
		cmd.Flags.StringVar(&cmd.streamPath, "stream-path", "", "Google Storage path to the stream protobuf.")

		return &cmd
	},
}

func (cmd *cmdRunTail) Run(baseApp subcommands.Application, args []string, _ subcommands.Env) int {
	app, c := getApplication(baseApp)

	switch {
	case cmd.indexPath == "":
		log.Errorf(c, "Missing required argument (-index-path).")
		return 1
	case cmd.streamPath == "":
		log.Errorf(c, "Missing required argument (-stream-path).")
		return 1
	}

	client, err := app.getGSClient(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create GS client.")
		return 1
	}
	defer client.Close()

	stClient, err := archive.New(c, archive.Options{
		Index:  gs.Path(cmd.indexPath),
		Stream: gs.Path(cmd.streamPath),
		Client: client,
	})
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create storage client.")
		return 1
	}
	defer stClient.Close()

	e, err := stClient.Tail("", "")
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to Tail log entries.")
		return 1
	}

	if e == nil {
		log.Infof(c, "No log data to tail.")
		return 0
	}

	le, err := e.GetLogEntry()
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to unmarshal log entry.")
		return 1
	}

	log.Fields{
		"index": le.StreamIndex,
		"size":  len(e.D),
	}.Debugf(c, "Dumping tail entry.")
	if err := unmarshalAndDump(c, os.Stdout, nil, le); err != nil {
		log.WithError(err).Errorf(c, "Failed to dump tail entry.")
		return 1
	}
	return 0
}

func dumpRecordIO(c context.Context, r io.Reader, cb func(context.Context, []byte) error) error {
	var (
		frameReader = recordio.NewReader(r, math.MaxInt64)
		frameIndex  = 0
		buf         bytes.Buffer
		eof         bool
	)
	for !eof {
		frameSize, r, err := frameReader.ReadFrame()
		switch err {
		case nil:
			break

		case io.EOF:
			log.Debugf(c, "Encountered EOF.")
			eof = true
			break

		default:
			return errors.Annotate(err).Reason("Encountered error reading log stream.").
				D("frameIndex", frameIndex).Err()
		}

		if frameSize > 0 {
			buf.Reset()
			buf.Grow(int(frameSize))

			if _, err := buf.ReadFrom(r); err != nil {
				return errors.Annotate(err).Reason("Failed to buffer frame.").
					D("frameIndex", frameIndex).Err()
			}

			if err := cb(c, buf.Bytes()); err != nil {
				return err
			}
		}

		log.Fields{
			"index": frameIndex,
			"size":  buf.Len(),
		}.Debugf(c, "Read frame.")
		frameIndex++
	}
	return nil
}
