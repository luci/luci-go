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
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"
	cloudkms "google.golang.org/api/cloudkms/v1"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

type commonFlags struct {
	subcommands.CommandRunBase
	authFlags      authcli.Flags
	parsedAuthOpts auth.Options
	output         string
}

func (c *commonFlags) Init(authOpts auth.Options) {
	c.authFlags.Register(&c.Flags, authOpts)
	c.Flags.StringVar(&c.output, "output", "", "Path to write operation results to (use '-' for stdout).")
}

func (c *commonFlags) Parse() error {
	var err error
	c.parsedAuthOpts, err = c.authFlags.Options()
	return err
}

func (c *commonFlags) createAuthClient(ctx context.Context) (*http.Client, error) {
	return auth.NewAuthenticator(ctx, auth.SilentLogin, c.parsedAuthOpts).Client()
}

func readInput(file string) ([]byte, error) {
	if file == "-" {
		return ioutil.ReadAll(os.Stdin)
	}
	return ioutil.ReadFile(file)
}

func writeOutput(file string, data []byte) error {
	if file == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return ioutil.WriteFile(file, data, 0664)
}

// cryptoKeysPathComponents are the path components necessary for API calls related to
// crypto keys.
//
// This structure represents the following path format:
// projects/.../locations/.../keyRings/.../cryptoKeys/...
var cryptoKeysPathComponents = []string{
	"projects",
	"locations",
	"keyRings",
	"cryptoKeys",
}

// validateCryptoKeysKMSPath validates a cloudkms path used for the API calls currently
// supported by this client.
//
// What this means is we only care about paths that look exactly like the ones
// constructed from kmsPathComponents.
func validateCryptoKeysKMSPath(path string) error {
	if path[0] == '/' {
		path = path[1:]
	}
	components := strings.Split(path, "/")
	if len(components) < (len(cryptoKeysPathComponents)-1)*2 || len(components) > len(cryptoKeysPathComponents)*2 {
		return errors.Reason("path should have the form %s", strings.Join(cryptoKeysPathComponents, "/.../")+"/...").Err()
	}
	for i, c := range components {
		if i%2 == 1 {
			continue
		}
		expect := cryptoKeysPathComponents[i/2]
		if c != expect {
			return errors.Reason("expected component %d to be %s, got %s", i+1, expect, c).Err()
		}
	}
	return nil
}

type cryptRun struct {
	commonFlags
	keyPath   string
	input     string
	doRequest func(ctx context.Context, service *cloudkms.Service, input []byte, keyPath string) ([]byte, error)
}

func (c *cryptRun) Init(authOpts auth.Options) {
	c.commonFlags.Init(authOpts)
	c.Flags.StringVar(&c.input, "input", "", "Path to file with data to operate on (use '-' for stdin). Data cannot be larger than 64KiB.")
}

func (c *cryptRun) Parse(ctx context.Context, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) < 1 {
		return errors.New("positional arguments missing")
	}
	if len(args) > 1 {
		return errors.New("unexpected positional arguments")
	}
	if c.input == "" {
		return errors.New("input file is required")
	}
	if c.output == "" {
		return errors.New("output location is required")
	}
	if err := validateCryptoKeysKMSPath(args[0]); err != nil {
		return err
	}
	c.keyPath = args[0]
	return nil
}

func (c *cryptRun) main(ctx context.Context) error {
	// Set up service.
	authCl, err := c.createAuthClient(ctx)
	if err != nil {
		return err
	}
	service, err := cloudkms.New(authCl)
	if err != nil {
		return err
	}

	// Read in input.
	bytes, err := readInput(c.input)
	if err != nil {
		return err
	}

	result, err := c.doRequest(ctx, service, bytes, c.keyPath)
	if err != nil {
		return err
	}

	// Write output.
	return writeOutput(c.output, result)
}

func (c *cryptRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, c, env)
	if err := c.Parse(ctx, args); err != nil {
		logging.WithError(err).Errorf(ctx, "Error while parsing arguments")
		return 1
	}
	if err := c.main(ctx); err != nil {
		logging.WithError(err).Errorf(ctx, "Error while executing command")
		return 1
	}
	return 0
}
