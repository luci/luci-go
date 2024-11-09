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

	cloudkms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/logging"
)

func doEncrypt(ctx context.Context, client *cloudkms.KeyManagementClient, input []byte, keyPath string) ([]byte, error) {
	// Set up request, encoding plaintext as base64.
	req := &kmspb.EncryptRequest{
		Name:      keyPath,
		Plaintext: input,
	}

	resp, err := client.Encrypt(ctx, req)
	if err != nil {
		logging.Errorf(ctx, "Error while making request")
		return nil, err
	}
	return resp.Ciphertext, nil
}

func cmdEncrypt(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "encrypt <options> <path>",
		ShortDesc: "encrypts some plaintext",
		LongDesc: `Uploads a plaintext for encryption by Cloud KMS.

<path> refers to the path to the crypto key. e.g.

projects/<project>/locations/<location>/keyRings/<keyRing>/cryptoKeys/<cryptoKey>`,
		CommandRun: func() subcommands.CommandRun {
			c := cryptRun{doCrypt: doEncrypt}
			c.Init(authOpts)
			return &c
		},
	}
}
