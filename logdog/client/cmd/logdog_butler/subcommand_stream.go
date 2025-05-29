// Copyright 2015 The LUCI Authors.
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
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/nestedflagset"
	log "go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/logdog/client/butler"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/logdog/common/types"
)

var subcommandStream = &subcommands.Command{
	UsageLine: "stream",
	ShortDesc: "Serves a single input stream.",
	LongDesc:  "Configures and transmits a single stream from the command-line.",
	CommandRun: func() subcommands.CommandRun {
		cmd := &streamCommandRun{}

		cmd.Flags.StringVar(&cmd.path, "source", "-",
			"The path of the file to stream. If omitted, STDIN will be used")

		streamFlag := &nestedflagset.FlagSet{}
		cmd.stream.addFlags(&streamFlag.F)
		cmd.Flags.Var(streamFlag, "stream", "Output stream parameters.")
		return cmd
	},
}

type streamCommandRun struct {
	subcommands.CommandRunBase

	path   string       // The stream input path.
	stream streamConfig // Stream configuration parameters.
}

// subcommands.Run
func (cmd *streamCommandRun) Run(app subcommands.Application, args []string, _ subcommands.Env) int {
	a := app.(*application)

	var (
		streamFile        *os.File
		defaultStreamName streamproto.StreamNameFlag
	)
	if cmd.path == "-" {
		defaultStreamName = "stdin"
		streamFile = os.Stdin
	} else {
		streamName, err := types.MakeStreamName("file:", cmd.path)
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
			}.Errorf(a, "Failed to generate stream name.")
			return runtimeErrorReturnCode
		}
		defaultStreamName = streamproto.StreamNameFlag(streamName)

		file, err := os.Open(cmd.path)
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"path":       cmd.path,
			}.Errorf(a, "Failed to open input stream file.")
			return runtimeErrorReturnCode
		}
		streamFile = file
	}
	if cmd.stream.Name == "" {
		cmd.stream.Name = defaultStreamName
	}

	// We think everything should work. Configure our Output instance.
	of, err := a.getOutputFactory()
	if err != nil {
		log.WithError(err).Errorf(a, "Failed to get output factory instance.")
		return runtimeErrorReturnCode
	}
	output, err := of.configOutput(a)
	if err != nil {
		log.WithError(err).Errorf(a, "Failed to create output instance.")
		return runtimeErrorReturnCode
	}
	defer output.Close()

	// Instantiate our Processor.
	err = a.runWithButler(output, func(b *butler.Butler) error {
		if err := b.AddStream(streamFile, cmd.stream.properties()); err != nil {
			return errors.Fmt("failed to add stream: %w", err)
		}

		b.Activate()
		return b.Wait()
	})
	if err != nil {
		logAnnotatedErr(a, err, "Failed to stream file.")
		return runtimeErrorReturnCode
	}

	return 0
}
