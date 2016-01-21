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

	"github.com/luci/luci-go/client/authcli"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/logging/gologger"
)

const (
	userAgent = "luci-rpc"
)

var logCfg = gologger.LoggerConfig{
	Format: `%{message}`,
	Out:    os.Stderr,
	Level:  gol.WARNING,
}

// cmdRun is a base of all rpc subcommands.
// It defines some common flags, such as logging and auth, and useful methods.
type cmdRun struct {
	subcommands.CommandRunBase
	verbose bool
	auth    authcli.Flags
}

// registerBaseFlags registers common flags used by all subcommands.
func (r *cmdRun) registerBaseFlags() {
	r.Flags.BoolVar(&r.verbose, "verbose", false, "Enable more logging.")
	r.auth.Register(&r.Flags, auth.Options{
		Logger: logCfg.Get(),
	})
}

// initContext creates a context. Must be called after flags are parsed.
func (r *cmdRun) initContext() (context.Context, error) {
	// Setup logger.
	logCfg := logCfg
	if r.verbose {
		logCfg.Level = gol.DEBUG
	}

	// Setup authenticated HTTP client.
	c := logCfg.Use(context.Background())
	authOpts, err := r.auth.Options()
	if err != nil {
		return nil, err
	}
	a := auth.NewAuthenticator(auth.OptionalLogin, authOpts)
	httpClient, err := a.Client()
	if err != nil {
		return nil, err
	}
	client := &clientImpl{*httpClient, a}
	c = context.WithValue(c, clientKey, client)

	return c, nil
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

// run initializes a context and runs f.
// if f returns an error, prints the error and returns a non-zero exit code.
func (r *cmdRun) run(f func(context.Context) error) int {
	ctx, err := r.initContext()
	if err != nil {
		return r.done(err)
	}
	return r.done(f(ctx))
}

var application = &subcommands.DefaultApplication{
	Name:  "rpc",
	Title: "Remote Procedure Call CLI",
	Commands: []*subcommands.Command{
		cmdCall,
		cmdShow,
		authcli.SubcommandLogin(auth.Options{Logger: logCfg.Get()}, "login"),
		authcli.SubcommandLogout(auth.Options{Logger: logCfg.Get()}, "logout"),
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
