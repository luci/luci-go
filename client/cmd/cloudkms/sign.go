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
	"io"
	"os"

	cloudkms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/logging"
)

func doSign(ctx context.Context, client *cloudkms.KeyManagementClient, input *os.File, keyPath string) ([]byte, error) {
	digest := sha256.New()
	if _, err := io.Copy(digest, input); err != nil {
		logging.Warningf(ctx, "Error while reading input")
		return nil, err
	}

	// Set up request, including a SHA256 hash of the plaintext.
	req := &kmspb.AsymmetricSignRequest{
		Name: keyPath,
		Digest: &kmspb.Digest{
			Digest: &kmspb.Digest_Sha256{
				Sha256: digest.Sum(nil),
			},
		},
	}

	resp, err := client.AsymmetricSign(ctx, req)
	if err != nil {
		logging.Errorf(ctx, "Error while making request")
		return nil, err
	}

	return resp.Signature, nil
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
