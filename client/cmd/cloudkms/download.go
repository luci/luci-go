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

	cloudkms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/logging"
)

func doDownload(ctx context.Context, client *cloudkms.KeyManagementClient, keyPath string) ([]byte, error) {
	resp, err := client.GetPublicKey(ctx, &kmspb.GetPublicKeyRequest{Name: keyPath})
	if err != nil {
		logging.Errorf(ctx, "Error while making request")
		return nil, err
	}

	return []byte(resp.Pem), nil
}

func cmdDownload(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "download <options> <path>",
		ShortDesc: "downloads public keys from Cloud KMS",
		LongDesc: `Downloads a public key from Cloud KMS.

<path> refers to the path to the crypto key. e.g.

projects/<project>/locations/<location>/keyRings/<keyRing>/cryptoKeys/<cryptoKey>/cryptoKeyVersions/<version>

-output will be the public key`,
		CommandRun: func() subcommands.CommandRun {
			c := downloadRun{doDownload: doDownload}
			c.Init(authOpts)
			return &c
		},
	}
}
