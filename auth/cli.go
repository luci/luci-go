// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"fmt"
	"net/http"
	"os"

	"github.com/maruel/subcommands"
)

// SubcommandLogin returns subcommands.Command that can be used to perform
// interactive login.
func SubcommandLogin(name string) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: name,
		ShortDesc: "performs interactive login flow",
		LongDesc:  "Performs interactive login flow and caches obtained credentials",
		CommandRun: func() subcommands.CommandRun {
			return &loginRun{}
		},
	}
}

type loginRun struct {
	subcommands.CommandRunBase
}

func (c *loginRun) Run(subcommands.Application, []string) int {
	transport, err := LoginIfRequired(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Login failed: %s\n", err.Error())
		return 1
	}
	err = reportIdentity(transport)
	if err != nil {
		return 1
	}
	return 0
}

// SubcommandLogout returns subcommands.Command that can be used to purge cached
// credentials.
func SubcommandLogout(name string) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: name,
		ShortDesc: "removes cached credentials",
		LongDesc:  "Removes cached credentials from the disk",
		CommandRun: func() subcommands.CommandRun {
			return &logoutRun{}
		},
	}
}

type logoutRun struct {
	subcommands.CommandRunBase
}

func (c *logoutRun) Run(a subcommands.Application, args []string) int {
	err := PurgeCredentialsCache()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// SubcommandInfo returns subcommand.Command that can be used to print current
// cached credentials.
func SubcommandInfo(name string) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: name,
		ShortDesc: "prints an email address associated with currently cached token",
		LongDesc:  "Prints an email address associated with currently cached token",
		CommandRun: func() subcommands.CommandRun {
			return &infoRun{}
		},
	}
}

type infoRun struct {
	subcommands.CommandRunBase
}

func (c *infoRun) Run(a subcommands.Application, args []string) int {
	transport, err := DefaultAuthenticator.Transport()
	if err == ErrLoginRequired {
		fmt.Fprintln(os.Stderr, "Not logged in")
		return 1
	} else if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	err = reportIdentity(transport)
	if err != nil {
		return 3
	}
	return 0
}

// reportIdentity prints identity associated with credentials that round tripper
// puts into each request (if any).
func reportIdentity(t http.RoundTripper) error {
	var ident Identity
	service, err := NewGroupsService("", &http.Client{Transport: t}, nil)
	if err == nil {
		ident, err = service.FetchCallerIdentity()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch current identity: %s\n", err)
		return err
	}
	fmt.Printf("Logged in to %s as %s\n", service.ServiceURL(), ident)
	return nil
}
