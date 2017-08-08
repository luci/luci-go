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

package main

import (
	"flag"
	"os"
	"os/signal"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/cli"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
)

type application struct {
	cli.Application

	layoutPath string
	workers    int

	layout deployLayout
	tools  tools
}

func (a *application) addFlags(fs *flag.FlagSet) {
	fs.StringVar(&a.layoutPath, "layout", a.layoutPath,
		"Path to the base layout file.")
	fs.IntVar(&a.workers, "workers", 0,
		"Maximum number of workers to use. If <= 0, no limit will be applied.")
}

func (a *application) runWork(c context.Context, fn func(*work) error) error {
	return runWork(c, a.workers, &a.tools, fn)
}

func mainImpl(args []string) int {
	c := gologger.StdConfig.Use(context.Background())
	a := application{
		Application: cli.Application{
			Name:    os.Args[0],
			Title:   "LUCI Deployment Tool",
			Context: func(context.Context) context.Context { return c },
			Commands: []*subcommands.Command{
				subcommands.CmdHelp,
				&cmdCheckout,
				&cmdDeploy,
				&cmdManage,
			},
		},
	}
	logFlags := log.Config{
		Level: log.Info,
	}

	var fs flag.FlagSet
	logFlags.AddFlags(&fs)
	a.addFlags(&fs)
	if err := fs.Parse(args); err != nil {
		log.WithError(err).Errorf(c, "Failed to parse flags.")
		return 1
	}
	c = logFlags.Set(c)

	// Install a signal handler to cancel everything in response to a Ctrl+C.
	var cancelFunc context.CancelFunc
	c, cancelFunc = context.WithCancel(c)
	signalC := make(chan os.Signal, 1)
	signal.Notify(signalC, os.Interrupt)
	go func(c context.Context) {
		signalled := false
		for range signalC {
			if !signalled {
				signalled = true

				// First '^C'; soft-terminate
				log.Infof(c, "Received Control-C (keyboard interrupt), shutting down. Interrupt again for immediate exit.")
				cancelFunc()
			} else {
				// Multiple '^C'; terminate immediately
				os.Exit(1)
			}
		}
	}(c)
	defer func() {
		signal.Stop(signalC)
		close(signalC)
	}()

	// Load our layout file.
	if err := a.layout.load(c, a.layoutPath); err != nil {
		logError(c, err, "Failed to load layout file from [%s]", a.layoutPath)
		return 1
	}

	// Run our subcommand (and parse subcommand flags).
	return subcommands.Run(&a, fs.Args())
}

func main() {
	os.Exit(mainImpl(os.Args[1:]))
}
