// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/maruel/subcommands"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	job "go.chromium.org/luci/led/job"
)

// TODO(iannucci): the 'subcommands' library is a mess, use something better.

type command interface {
	subcommands.CommandRun

	initFlags(authOpts auth.Options)

	jobInput() bool
	positionalRange() (min, max int)

	validateFlags(ctx context.Context, positionals []string, env subcommands.Env) error
	execute(ctx context.Context, authClient *http.Client, inJob *job.Definition) (output interface{}, err error)
}

type cmdBase struct {
	subcommands.CommandRunBase

	logFlags  logging.Config
	authFlags authcli.Flags

	authenticator *auth.Authenticator
}

func (c *cmdBase) initFlags(authOpts auth.Options) {
	c.logFlags.Level = logging.Info
	c.logFlags.AddFlags(&c.Flags)
	c.authFlags.Register(&c.Flags, authOpts)
}

func readJobDefinition(ctx context.Context) (*job.Definition, error) {
	done := uint32(0)
	go func() {
		time.Sleep(time.Second)
		if atomic.LoadUint32(&done) == 0 {
			logging.Warningf(ctx, "waiting for JobDefinition on stdin...")
		}
	}()

	jd := &job.Definition{}
	err := jsonpb.Unmarshal(os.Stdin, jd)
	atomic.StoreUint32(&done, 1)
	return jd, errors.Annotate(err, "decoding job Definition").Err()
}

func (c *cmdBase) doContextExecute(a subcommands.Application, cmd command, args []string, env subcommands.Env) int {
	ctx := c.logFlags.Set(cli.GetContext(a, cmd, env))
	authOpts, err := c.authFlags.Options()
	if err != nil {
		logging.Errorf(ctx, "bad auth arguments: %s\n\n", err)
		c.GetFlags().Usage()
		return 1
	}
	c.authenticator = auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts)
	authClient, err := c.authenticator.Client()
	if err == auth.ErrLoginRequired {
		fmt.Fprintln(os.Stderr, "Login required: run `led auth-login`.")
		return 1
	}

	//positional
	min, max := cmd.positionalRange()
	if len(args) < min {
		logging.Errorf(ctx, "expected at least %d positional arguments, got %d", min, len(args))
		c.GetFlags().Usage()
		return 1
	}
	if len(args) > max {
		logging.Errorf(ctx, "expected at most %d positional arguments, got %d", max, len(args))
		c.GetFlags().Usage()
		return 1
	}

	if err = cmd.validateFlags(ctx, args, env); err != nil {
		logging.Errorf(ctx, "bad arguments: %s\n\n", err)
		c.GetFlags().Usage()
		return 1
	}

	var inJob *job.Definition
	if cmd.jobInput() {
		if inJob, err = readJobDefinition(ctx); err != nil {
			errors.Log(ctx, err)
			return 1
		}
	}

	output, err := cmd.execute(ctx, authClient, inJob)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}

	if output != nil {
		switch x := output.(type) {
		case proto.Message:
			err = (&jsonpb.Marshaler{
				OrigName: true,
				Indent:   "  ",
			}).Marshal(os.Stdout, x)

		default:
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			err = enc.Encode(output)
		}
		if err != nil {
			errors.Log(ctx, errors.Annotate(err, "encoding output").Err())
			return 1
		}
	}

	return 0
}

func validateHost(host string) error {
	if rsp, err := http.Get("https://" + host); err != nil {
		return errors.Annotate(err, "%q", host).Err()
	} else if rsp.StatusCode != 200 {
		return errors.Reason("%q: bad status %d", host, rsp.StatusCode).Err()
	}
	return nil
}
