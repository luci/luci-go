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

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
)

func doEncrypt(ctx context.Context, service *cloudkms.Service, input []byte, keyPath string) ([]byte, error) {
	// Set up request, encoding plaintext as base64.
	req := cloudkms.EncryptRequest{
		Plaintext: base64.StdEncoding.EncodeToString(input),
	}

	var resp *cloudkms.EncryptResponse
	err := retry.Retry(ctx, transient.Only(retry.Default), func() error {
		var err error
		resp, err = service.
			Projects.
			Locations.
			KeyRings.
			CryptoKeys.
			Encrypt(keyPath, &req).Context(ctx).Do()
		return err
	}, func(err error, d time.Duration) {
		logging.Warningf(ctx, "Transient error while making request, retrying in %s...", d)
	})
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(resp.Ciphertext)
}

func cmdEncrypt(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "encrypt <options> <path>",
		ShortDesc: "encrypts some plaintext",
		LongDesc: `Uploads a plaintext for encryption by cloudkms.

<path> refers to the path to the crypto key. e.g.

projects/<project>/locations/<location>/keyRings/<keyRing>/cryptoKeys/<cryptoKey>`,
		CommandRun: func() subcommands.CommandRun {
			c := cryptRun{doRequest: doEncrypt}
			c.Init(authOpts)
			return &c
		},
	}
}
