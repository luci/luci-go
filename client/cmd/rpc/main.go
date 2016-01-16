// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/maruel/subcommands"
	gol "github.com/op/go-logging"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/logging/gologger"
)

const (
	userAgent = "luci-rpc"
)

// cmdRun is a base of all rpc subcommands.
// It defines some common flags, such as logging, and useful methods.
type cmdRun struct {
	subcommands.CommandRunBase
	verbose bool
}

// registerBaseFlags registers common flags used by all subcommands.
func (r *cmdRun) registerBaseFlags() {
	r.Flags.BoolVar(&r.verbose, "verbose", false, "Enable more logging.")
}

// initContext creates a context with installed logger.
func (r *cmdRun) initContext() context.Context {
	// Setup logger.
	loggerConfig := gologger.LoggerConfig{
		Format: `%{message}`,
		Out:    os.Stderr,
		Level:  gol.WARNING,
	}
	if r.verbose {
		loggerConfig.Level = gol.DEBUG
	}
	return loggerConfig.Use(context.Background())
}

// argErr prints an err and usage to stderr and returns an exit code.
func (r *cmdRun) argErr(format string, a ...interface{}) int {
	if format != "" {
		fmt.Fprintf(os.Stderr, format+"\n", a...)
	}
	fmt.Fprintln(os.Stderr, cmdCall.ShortDesc)
	fmt.Fprintln(os.Stderr, cmdCall.UsageLine)
	fmt.Fprintln(os.Stderr, "\nFlags:")
	r.Flags.PrintDefaults()
	return 1
}

// done prints err to stderr if it is not nil and returns an exit code.
func (r *cmdRun) done(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return 0
}

var application = &subcommands.DefaultApplication{
	Name:  "rpc",
	Title: "Remote Procedure Call CLI",
	Commands: []*subcommands.Command{
		cmdCall,
		cmdShow,
	},
}

func main() {
	os.Exit(subcommands.Run(application, os.Args[1:]))
}

// parseServer validates and parses a server URL.
func parseServer(host string) (*url.URL, error) {
	host = strings.TrimSuffix(host, "/")

	switch {
	case host == "":
		return nil, fmt.Errorf("unspecified")

	case strings.Contains(host, "://"):
		return nil, fmt.Errorf("must not have scheme")

	case strings.ContainsAny(host, "?/#"):
		return nil, fmt.Errorf("must not have query, path or fragment")

	case strings.HasPrefix(host, ":"):
		host = "localhost" + host
	}

	u := &url.URL{
		Scheme: "https",
		Host:   host,
	}
	if isLocalHost(host) {
		u.Scheme = "http"
	}
	return u, nil
}

func isLocalHost(host string) bool {
	return host == "localhost" || strings.HasPrefix(host, "localhost:") ||
		host == "127.0.0.1" || strings.HasPrefix(host, "127.0.0.1:")
}
