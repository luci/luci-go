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
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/client/butler"
	"go.chromium.org/luci/logdog/client/butler/streamserver"

	"github.com/maruel/subcommands"
)

var subcommandServe = &subcommands.Command{
	UsageLine: "serve",
	ShortDesc: "Instantiates a stream server.",
	LongDesc:  "Instantiates a stream server, accepting connections and forwarding them to output.",
	CommandRun: func() subcommands.CommandRun {
		cmd := &serveCommandRun{}

		return cmd
	},
}

type serveCommandRun struct {
	subcommands.CommandRunBase
}

func (cmd *serveCommandRun) Run(app subcommands.Application, args []string, _ subcommands.Env) int {
	a := app.(*application)

	streamServer, err := streamserver.New(a, "")
	if err != nil {
		log.WithError(err).Errorf(a, "Failed to make to stream server.")
		return runtimeErrorReturnCode
	}

	if err := streamServer.Listen(); err != nil {
		log.WithError(err).Errorf(a, "Failed to connect to stream server.")
		return runtimeErrorReturnCode
	}
	log.Infof(a, "listening on %q", streamServer.Address())

	// We think everything will work. Configure our Output instance.
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

	err = a.runWithButler(output, func(b *butler.Butler) error {
		b.AddStreamServer(streamServer)
		return b.Wait()
	})
	if err != nil {
		logAnnotatedErr(a, err, "Failed to serve.")
		return runtimeErrorReturnCode
	}

	return 0
}
