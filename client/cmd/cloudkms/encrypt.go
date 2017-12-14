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

	"github.com/maruel/subcommands"
	cloudkms "google.golang.org/api/cloudkms/v1"

	"go.chromium.org/luci/common/errors"
)

func cmdEncrypt(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "encrypt <options> <name>",
		ShortDesc: "encrypts some plaintext",
		LongDesc: `Uploads a plaintext for encryption by cloudkms.

<name> refers to the path to the crypto key. e.g. <project>/<location>/<keyRing>/<cryptoKey>`
		CommandRun: func() subcommands.CommandRun {
			c := encryptRun{}
			c.commonFlags.Init(authOpts)
			c.Flags.StringVar(&c.input, "input", "", "Input plaintext to encrypt.")
			return &c
		},
	}
}

type encryptRun struct {
	commonFlags
	input string
}

func (c *encryptRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) < 1 {
		return errors.New("position arguments missing")
	}
	if len(args) > 1 {
		return errors.New("position arguments not expected")
	}
	if c.input == "" {
		return errors.New("input plaintext file is required")
	}
	if c.output == "" {
		return errors.New("output location is required")
	}
	return nil
}

func (c *encryptRun) main(a subcommands.Application, args []string) error {
	authCl, err := c.createAuthClient()
	if err != nil {
		return err
	}
	ctx := c.defaultFlags.MakeLoggingContext(os.Stderr)

	service, err := cloudkms.New(authCl)
	if err != nil {
		return err
	}

	bytes, err := ioutil.ReadFile(c.input)
	if err != nil {
		return err
	}
	req := cloudkms.EncryptRequest{Plaintext: string(bytes)}
	resp, err := service.Projects.Locations.KeyRings.CryptoKeys.Encrypt(args[0], req)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(c.output, []byte(resp.Ciphertext), 0664)
}

func (c *encryptRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if err := c.Parse(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if err := c.main(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
