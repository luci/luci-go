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
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"os"

	cloudkms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/logging"
)

func doVerify(ctx context.Context, client *cloudkms.KeyManagementClient, input *os.File, inputSig []byte, keyPath string) error {
	// Verify Elliptic Curve signature.
	var parsedSig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(inputSig, &parsedSig); err != nil {
		return fmt.Errorf("asn1.Unmarshal: %v", err)
	}

	resp, err := client.GetPublicKey(ctx, &kmspb.GetPublicKeyRequest{Name: keyPath})
	if err != nil {
		logging.Errorf(ctx, "Error while making request")
		return err
	}

	// Parse the key.
	block, _ := pem.Decode([]byte(resp.Pem))
	abstractKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("x509.ParsePKIXPublicKey: %v", err)
	}
	ecKey, ok := abstractKey.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("key '%s' is not EC", keyPath)
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, input); err != nil {
		logging.Errorf(ctx, "Error while hashing input")
		return err
	}
	digest := hash.Sum(nil)
	if !ecdsa.Verify(ecKey, digest, parsedSig.R, parsedSig.S) {
		return fmt.Errorf("ecdsa.Verify failed on key: %s", keyPath)
	}

	return nil
}

func cmdVerify(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "verify <options> <key_path>",
		ShortDesc: "verify a signature that was previously created using a key in Cloud KMS",
		LongDesc: `Verify a signature that was previously created with a key stored in Cloud KMS.

At this time, this tool assumes that all signatures are on SHA-256 digests and
all keys are EC_SIGN_P256_SHA256.

<key_path> refers to the path to the public crypto key. e.g.

projects/<project>/locations/<location>/keyRings/<keyRing>/cryptoKeys/<cryptoKey>/cryptoKeyVersions/<version>

Exit code will be 0 if verification successful`,
		CommandRun: func() subcommands.CommandRun {
			c := verifyRun{doVerify: doVerify}
			c.Init(authOpts)
			return &c
		},
	}
}
