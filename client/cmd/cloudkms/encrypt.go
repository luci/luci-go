// Copyright 2017 The LUCI Authors.
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
	"encoding/base64"
	"time"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"
	cloudkms "google.golang.org/api/cloudkms/v1"

	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
)

func cmdEncrypt(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "encrypt <options> <path>",
		ShortDesc: "encrypts some plaintext",
		LongDesc: `Uploads a plaintext for encryption by cloudkms.

<path> refers to the path to the crypto key. e.g. <project>/<location>/<keyRing>/<cryptoKey>`,
		CommandRun: func() subcommands.CommandRun {
			c := encryptRun{}
			c.commonFlags.Init(authOpts)
			c.Flags.StringVar(&c.input, "input", "", "Path to read in plaintext from (use '-' for stdin). Data cannot be larger than 64KiB.")
			return &c
		},
	}
}

type encryptRun struct {
	commonFlags
	keyPath string
	input   string
}

func (c *encryptRun) Parse(ctx context.Context, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) < 1 {
		return errors.New("positional arguments missing")
	}
	if len(args) > 1 {
		return errors.New("positional arguments not expected")
	}
	if c.input == "" {
		return errors.New("input plaintext file is required")
	}
	if c.output == "" {
		return errors.New("output location is required")
	}
	keyPath, err := parseKMSPath(args[0])
	if err != nil {
		return err
	}
	c.keyPath = keyPath
	return nil
}

func (c *encryptRun) main(ctx context.Context) error {
	// Set up service.
	authCl, err := c.createAuthClient(ctx)
	if err != nil {
		return err
	}
	service, err := cloudkms.New(authCl)
	if err != nil {
		return err
	}

	// Read in plaintext.
	bytes, err := readInput(c.input)
	if err != nil {
		return err
	}

	// Set up request, encoding plaintext as base64.
	req := cloudkms.EncryptRequest{
		Plaintext: base64.StdEncoding.EncodeToString(bytes),
	}

	var resp *cloudkms.EncryptResponse
	err = retry.Retry(ctx, transient.Only(retry.Default), func() error {
		var err error
		resp, err = service.
			Projects.
			Locations.
			KeyRings.
			CryptoKeys.
			Encrypt(c.keyPath, &req).Context(ctx).Do()
		return err
	}, func(err error, d time.Duration) {
		logging.Warningf(ctx, "Transient error while making request, retrying in %s...", d)
	})
	if err != nil {
		return err
	}

	// Write ciphertext out.
	return writeOutput(c.output, []byte(resp.Ciphertext))
}

func (c *encryptRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, c, env)
	if err := c.Parse(ctx, args); err != nil {
		logging.WithError(err).Errorf(ctx, "Error while parsing arguments")
		return 1
	}
	if err := c.main(ctx); err != nil {
		logging.WithError(err).Errorf(ctx, "Error while making encrypt request")
		return 1
	}
	return 0
}
