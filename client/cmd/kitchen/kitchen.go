// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"
	"os/signal"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
)

var application = &cli.Application{
	Name:  "kitchen",
	Title: "Kitchen. It can run a recipe.",
	Context: func(ctx context.Context) context.Context {
		ctx = gologger.StdConfig.Use(ctx)
		return handleInterruption(ctx)
	},
	Commands: []*subcommands.Command{
		subcommands.CmdHelp,
		cmdCook,
	},
}

func main() {
	os.Exit(subcommands.Run(application, os.Args[1:]))
}

func handleInterruption(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	signalC := make(chan os.Signal)
	signal.Notify(signalC, os.Interrupt)
	go func() {
		interrupted := false
		for range signalC {
			if interrupted {
				os.Exit(1)
			}
			interrupted = true
			cancel()
		}
	}()
	return ctx
}

// logAnnotatedErr logs the full stack trace from an annotated error to the
// installed logger at error level.
func logAnnotatedErr(ctx context.Context, err error) {
	if err == nil {
		return
	}

	var buf bytes.Buffer
	st := errors.RenderStack(err)
	if _, derr := st.DumpTo(&buf); derr != nil {
		// This can't really fail, since we're rendering to a Buffer.
		panic(derr)
	}

	log.Errorf(ctx, "Annotated error stack:\n%s", buf.String())
}
