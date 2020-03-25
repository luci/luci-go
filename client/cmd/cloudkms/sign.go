// Copyright 2020 The LUCI Authors.
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
	"crypto/sha256"
	"encoding/base64"
	"io"
	"os"
	"time"

	"github.com/maruel/subcommands"

	cloudkms "google.golang.org/api/cloudkms/v1"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
)

func doSign(ctx context.Context, service *cloudkms.Service, input *os.File, keyPath string) ([]byte, error) {
	digest := sha256.New()
	if _, err := io.Copy(digest, input); err != nil {
		logging.Warningf(ctx, "Error while reading input")
		return nil, err
	}
	digestB64 := base64.StdEncoding.EncodeToString(digest.Sum(nil))

	// Set up request, including a SHA256 hash of the plaintext.
	req := cloudkms.AsymmetricSignRequest{
		Digest: &cloudkms.Digest{Sha256: digestB64},
	}

	var resp *cloudkms.AsymmetricSignResponse
	err := retry.Retry(ctx, transient.Only(retry.Default), func() error {
		var err error
		resp, err = service.
			Projects.
			Locations.
			KeyRings.
			CryptoKeys.
			CryptoKeyVersions.
			AsymmetricSign(keyPath, &req).Context(ctx).Do()
		return err
	}, func(err error, d time.Duration) {
		logging.Warningf(ctx, "Transient error while making request, retrying in %s...", d)
	})
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(resp.Signature)
}

func cmdSign(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "sign <options> <path>",
		ShortDesc: "signs some plaintext",
		LongDesc: `Processes a plaintext and uploads the digest for signing by Cloud KMS.

At this time, this tool assumes that all signatures are on SHA-256 digests and
all keys are EC_SIGN_P256_SHA256.

<path> refers to the path to the crypto key. e.g.

projects/<project>/locations/<location>/keyRings/<keyRing>/cryptoKeys/<cryptoKey>/cryptoKeyVersions/<version>

-output will be the signature`,
		CommandRun: func() subcommands.CommandRun {
			c := signRun{doSign: doSign}
			c.Init(authOpts)
			return &c
		},
	}
}
