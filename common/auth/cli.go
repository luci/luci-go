// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/maruel/subcommands"
)

// Flags defines command line flags related to authentication.
type Flags struct {
	defaults           Options
	serviceAccountJSON string
}

// Register adds auth related flags to a FlagSet.
func (fl *Flags) Register(f *flag.FlagSet) {
	f.StringVar(&fl.serviceAccountJSON, "service-account-json", "", "Path to JSON file with service account credentials to use.")
}

// Options return instance of Options struct with values set accordingly to
// parsed command line flags.
func (fl *Flags) Options() (Options, error) {
	opts := fl.defaults
	if fl.serviceAccountJSON != "" {
		opts.Method = ServiceAccountMethod
		opts.ServiceAccountJSONPath = fl.serviceAccountJSON
	}
	return opts, nil
}

// SubcommandLogin returns subcommands.Command that can be used to perform
// interactive login.
func SubcommandLogin(opts Options, name string) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: name,
		ShortDesc: "performs interactive login flow",
		LongDesc:  "Performs interactive login flow and caches obtained credentials",
		CommandRun: func() subcommands.CommandRun {
			c := &loginRun{}
			c.flags.defaults = opts
			c.flags.Register(&c.Flags)
			return c
		},
	}
}

type loginRun struct {
	subcommands.CommandRunBase
	flags Flags
}

func (c *loginRun) Run(subcommands.Application, []string) int {
	opts, err := c.flags.Options()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return 1
	}
	client, err := AuthenticatedClient(InteractiveLogin, NewAuthenticator(opts))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Login failed: %s\n", err.Error())
		return 2
	}
	err = reportIdentity(client)
	if err != nil {
		return 3
	}
	return 0
}

// SubcommandLogout returns subcommands.Command that can be used to purge cached
// credentials.
func SubcommandLogout(opts Options, name string) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: name,
		ShortDesc: "removes cached credentials",
		LongDesc:  "Removes cached credentials from the disk",
		CommandRun: func() subcommands.CommandRun {
			c := &logoutRun{}
			c.flags.defaults = opts
			c.flags.Register(&c.Flags)
			return c
		},
	}
}

type logoutRun struct {
	subcommands.CommandRunBase
	flags Flags
}

func (c *logoutRun) Run(a subcommands.Application, args []string) int {
	opts, err := c.flags.Options()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return 1
	}
	err = NewAuthenticator(opts).PurgeCredentialsCache()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return 0
}

// SubcommandInfo returns subcommand.Command that can be used to print current
// cached credentials.
func SubcommandInfo(opts Options, name string) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: name,
		ShortDesc: "prints an email address associated with currently cached token",
		LongDesc:  "Prints an email address associated with currently cached token",
		CommandRun: func() subcommands.CommandRun {
			c := &infoRun{}
			c.flags.defaults = opts
			c.flags.Register(&c.Flags)
			return c
		},
	}
}

type infoRun struct {
	subcommands.CommandRunBase
	flags Flags
}

func (c *infoRun) Run(a subcommands.Application, args []string) int {
	opts, err := c.flags.Options()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		return 1
	}
	client, err := AuthenticatedClient(SilentLogin, NewAuthenticator(opts))
	if err == ErrLoginRequired {
		fmt.Fprintln(os.Stderr, "Not logged in")
		return 2
	} else if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 3
	}
	err = reportIdentity(client)
	if err != nil {
		return 4
	}
	return 0
}

// reportIdentity prints identity associated with credentials that the client
// puts into each request (if any).
func reportIdentity(c *http.Client) error {
	service := NewGroupsService("", c, nil)
	ident, err := service.FetchCallerIdentity()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch current identity: %s\n", err)
		return err
	}
	fmt.Printf("Logged in to %s as %s\n", service.ServiceURL(), ident)
	return nil
}
