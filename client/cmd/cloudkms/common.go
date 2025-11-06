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
	"context"
	"io"
	"os"
	"strings"

	cloudkms "cloud.google.com/go/kms/apiv1"
	"github.com/maruel/subcommands"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"

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
	keyPath        string
}

func (c *commonFlags) Init(authOpts auth.Options) {
	c.authFlags.Register(&c.Flags, authOpts)
}

func (c *commonFlags) Parse(args []string) error {
	var err error
	c.parsedAuthOpts, err = c.authFlags.Options()
	if err != nil {
		return err
	}

	if len(args) < 1 {
		return errors.New("positional arguments missing")
	}
	if len(args) > 1 {
		return errors.New("unexpected positional arguments")
	}
	if err := validateCryptoKeysKMSPath(args[0]); err != nil {
		return err
	}
	c.keyPath = args[0]

	return nil
}

func (c *commonFlags) createAuthTokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	a := auth.NewAuthenticator(ctx, auth.SilentLogin, c.parsedAuthOpts)
	if err := a.CheckLoginRequired(); err != nil {
		return nil, errors.Fmt("please login with\n$ %s", c.parsedAuthOpts.LoginCommandHint())
	}
	return a.TokenSource()
}

func (c *commonFlags) commonMain(ctx context.Context) (*cloudkms.KeyManagementClient, error) {
	// Set up service.
	authTS, err := c.createAuthTokenSource(ctx)
	if err != nil {
		return nil, err
	}
	client, err := cloudkms.NewKeyManagementClient(ctx, option.WithTokenSource(authTS))
	if err != nil {
		return nil, err
	}

	return client, nil
}

func readInput(file string) ([]byte, error) {
	if file == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(file)
}

func readInputFd(file string) (*os.File, error) {
	if file == "-" {
		return os.Stdin, nil
	}
	return os.Open(file)
}

func writeOutput(file string, data []byte) error {
	if file == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(file, data, 0664)
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
	"cryptoKeyVersions",
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
		return errors.Fmt("path should have the form %s", strings.Join(cryptoKeysPathComponents, "/.../")+"/...")
	}
	for i, c := range components {
		if i%2 == 1 {
			continue
		}
		expect := cryptoKeysPathComponents[i/2]
		if c != expect {
			return errors.Fmt("expected component %d to be %s, got %s", i+1, expect, c)
		}
	}
	return nil
}

type verifyRun struct {
	commonFlags
	input    string
	inputSig string
	doVerify func(ctx context.Context, client *cloudkms.KeyManagementClient, input *os.File, inputSig []byte, keyPath string) error
}

func (v *verifyRun) Init(authOpts auth.Options) {
	v.commonFlags.Init(authOpts)
	v.Flags.StringVar(&v.input, "input", "", "Path to file with data to verify (use '-' for stdin).")
	v.Flags.StringVar(&v.inputSig, "input-sig", "", "Path to read signature from (use '-' for stdin).")
}

func (v *verifyRun) Parse(ctx context.Context, args []string) error {
	if err := v.commonFlags.Parse(args); err != nil {
		return err
	}
	if v.input == "" {
		return errors.New("input file is required")
	}
	if v.inputSig == "" {
		return errors.New("input signature is required")
	}
	return nil
}

func (v *verifyRun) main(ctx context.Context) error {
	service, err := v.commonMain(ctx)
	if err != nil {
		return err
	}

	// Open input file descriptor.
	fd, err := readInputFd(v.input)
	if err != nil {
		return err
	}
	defer fd.Close()

	// Read in signature.
	sigBytes, err := readInput(v.inputSig)
	if err != nil {
		return err
	}

	return v.doVerify(ctx, service, fd, sigBytes, v.keyPath)
}

func (v *verifyRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, v, env)
	if err := v.Parse(ctx, args); err != nil {
		logging.WithError(err).Errorf(ctx, "Error while parsing arguments")
		return 1
	}
	if err := v.main(ctx); err != nil {
		logging.WithError(err).Errorf(ctx, "Error while executing command")
		return 1
	}
	return 0
}

type signRun struct {
	commonFlags
	input  string
	output string
	doSign func(ctx context.Context, client *cloudkms.KeyManagementClient, input *os.File, keyPath string) ([]byte, error)
}

func (s *signRun) Init(authOpts auth.Options) {
	s.commonFlags.Init(authOpts)
	s.Flags.StringVar(&s.input, "input", "", "Path to file with data to sign (use '-' for stdin).")
	s.Flags.StringVar(&s.output, "output", "", "Path to write signature to (use '-' for stdout).")
}

func (s *signRun) Parse(ctx context.Context, args []string) error {
	if err := s.commonFlags.Parse(args); err != nil {
		return err
	}
	if s.input == "" {
		return errors.New("input file is required")
	}
	if s.output == "" {
		return errors.New("output location is required")
	}
	return nil
}

func (s *signRun) main(ctx context.Context) error {
	service, err := s.commonMain(ctx)
	if err != nil {
		return err
	}

	// Read in input.
	fd, err := readInputFd(s.input)
	if err != nil {
		return err
	}
	defer fd.Close()

	result, err := s.doSign(ctx, service, fd, s.keyPath)
	if err != nil {
		return err
	}

	// Write output.
	return writeOutput(s.output, result)
}

func (s *signRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, s, env)
	if err := s.Parse(ctx, args); err != nil {
		logging.WithError(err).Errorf(ctx, "Error while parsing arguments")
		return 1
	}
	if err := s.main(ctx); err != nil {
		logging.WithError(err).Errorf(ctx, "Error while executing command")
		return 1
	}
	return 0
}

type cryptRun struct {
	commonFlags
	input   string
	output  string
	doCrypt func(ctx context.Context, client *cloudkms.KeyManagementClient, input []byte, keyPath string) ([]byte, error)
}

func (c *cryptRun) Init(authOpts auth.Options) {
	c.commonFlags.Init(authOpts)
	c.Flags.StringVar(&c.input, "input", "", "Path to file with data to operate on (use '-' for stdin). Data for encrypt and decrypt cannot be larger than 64KiB.")
	c.Flags.StringVar(&c.output, "output", "", "Path to write operation results to (use '-' for stdout).")
}

func (c *cryptRun) Parse(ctx context.Context, args []string) error {
	if err := c.commonFlags.Parse(args); err != nil {
		return err
	}
	if c.input == "" {
		return errors.New("input file is required")
	}
	if c.output == "" {
		return errors.New("output location is required")
	}
	return nil
}

func (c *cryptRun) main(ctx context.Context) error {
	service, err := c.commonMain(ctx)
	if err != nil {
		return err
	}

	// Read in input.
	bytes, err := readInput(c.input)
	if err != nil {
		return err
	}

	result, err := c.doCrypt(ctx, service, bytes, c.keyPath)
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

type downloadRun struct {
	commonFlags
	output     string
	doDownload func(ctx context.Context, client *cloudkms.KeyManagementClient, keyPath string) ([]byte, error)
}

func (d *downloadRun) Init(authOpts auth.Options) {
	d.commonFlags.Init(authOpts)
	d.Flags.StringVar(&d.output, "output", "", "Path to write key to (use '-' for stdout).")
}

func (d *downloadRun) Parse(ctx context.Context, args []string) error {
	if err := d.commonFlags.Parse(args); err != nil {
		return err
	}
	if d.output == "" {
		return errors.New("output location is required")
	}
	return nil
}

func (d *downloadRun) main(ctx context.Context) error {
	service, err := d.commonMain(ctx)
	if err != nil {
		return err
	}

	result, err := d.doDownload(ctx, service, d.keyPath)
	if err != nil {
		return err
	}

	// Write output.
	return writeOutput(d.output, result)
}

func (d *downloadRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, d, env)
	if err := d.Parse(ctx, args); err != nil {
		logging.WithError(err).Errorf(ctx, "Error while parsing arguments")
		return 1
	}
	if err := d.main(ctx); err != nil {
		logging.WithError(err).Errorf(ctx, "Error while executing command")
		return 1
	}
	return 0
}
