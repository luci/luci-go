// Copyright 2024 The LUCI Authors.
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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	minterpb "go.chromium.org/luci/tokenserver/api/minter/v1"
)

func TestMain(t *testing.T) {
	tmp := t.TempDir()
	tmpPath := func(file string) string {
		return filepath.Join(tmp, file)
	}
	readTmp := func(file string, body any) error {
		blob, err := os.ReadFile(tmpPath(file))
		if err != nil {
			return err
		}
		return json.Unmarshal(blob, body)
	}

	// Generate a private key and a self-signed certificate. In reality the
	// certificate is signed by some CA, but using a self-signed key is simpler
	// in tests.
	const machineCN = "test.example.com"
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: machineCN},
		DNSNames:              []string{machineCN},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}

	writePEM := func(path, typ string, bytes []byte) {
		f, err := os.Create(tmpPath(path))
		if err != nil {
			t.Fatal(err)
		}
		if err := pem.Encode(f, &pem.Block{Type: typ, Bytes: bytes}); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}

	writePEM("pk.pem", "RSA PRIVATE KEY", privBytes)
	writePEM("cert.pem", "CERTIFICATE", certBytes)

	// Launch a local token server to use in the test. Note that this uses a real
	// pRPC client. It doesn't work with mocked time. We'll use real time in the
	// test.
	ctx := context.Background()
	srv := prpctest.Server{}
	minterpb.RegisterTokenMinterServer(&srv, &mockTokenMinterServer{})
	srv.Start(ctx)
	defer srv.Close()

	run := func(args ...string) {
		code := realMain(ctx, append([]string{
			"-ts-mon-endpoint", "file://",
			"-pkey-pem", tmpPath("pk.pem"),
			"-cert-pem", tmpPath("cert.pem"),
			"-backend", srv.Host,
			"-token-file", tmpPath("token.json"),
			"-token-file-copy", tmpPath("copy1.json"),
			"-token-file-copy", tmpPath("copy2.json"),
			"-status-file", tmpPath("status.json"),
		}, args...))
		assert.That(t, code, should.Equal(0))
	}

	// The initial run generates the token.
	run()
	var status Report
	assert.That(t, readTmp("status.json", &status), should.ErrLike(nil))
	assert.That(t, status.UpdateReason, should.Equal("NEW_TOKEN"))
	assert.That(t, status.UpdateOutcome, should.Equal("UPDATE_SUCCESS"))

	// Created all 3 token files.
	var tok struct {
		LuciMachineToken string `json:"luci_machine_token"`
	}
	for _, p := range []string{"token.json", "copy1.json", "copy2.json"} {
		assert.That(t, readTmp(p, &tok), should.ErrLike(nil))
		assert.That(t, tok.LuciMachineToken, should.Equal("token-1"))
	}

	// Running again does nothing at all.
	run()
	assert.That(t, readTmp("status.json", &status), should.ErrLike(nil))
	assert.That(t, status.UpdateReason, should.Equal("TOKEN_IS_GOOD"))
	assert.That(t, status.UpdateOutcome, should.Equal("TOKEN_IS_GOOD"))

	// If some of the copies are missing, recreates them.
	assert.That(t, os.Remove(tmpPath("copy2.json")), should.ErrLike(nil))
	run()
	assert.That(t, readTmp("status.json", &status), should.ErrLike(nil))
	assert.That(t, status.UpdateReason, should.Equal("MISSING_TOKEN_COPY"))
	assert.That(t, status.UpdateOutcome, should.Equal("TOKEN_IS_GOOD"))
	assert.That(t, readTmp("copy2.json", &tok), should.ErrLike(nil))
	assert.That(t, tok.LuciMachineToken, should.Equal("token-1"))

	// Force run refreshes the token.
	run("-force-refresh")
	for _, p := range []string{"token.json", "copy1.json", "copy2.json"} {
		assert.That(t, readTmp(p, &tok), should.ErrLike(nil))
		assert.That(t, tok.LuciMachineToken, should.Equal("token-2"))
	}
}

type mockTokenMinterServer struct {
	minterpb.UnimplementedTokenMinterServer

	counter int
}

func (s *mockTokenMinterServer) MintMachineToken(ctx context.Context, req *minterpb.MintMachineTokenRequest) (*minterpb.MintMachineTokenResponse, error) {
	s.counter++
	return &minterpb.MintMachineTokenResponse{
		TokenResponse: &minterpb.MachineTokenResponse{
			ServiceVersion: "mocked-v1",
			TokenType: &minterpb.MachineTokenResponse_LuciMachineToken{
				LuciMachineToken: &minterpb.LuciMachineToken{
					MachineToken: fmt.Sprintf("token-%d", s.counter),
					Expiry:       timestamppb.New(clock.Now(ctx).Add(time.Hour)),
				},
			},
		},
	}, nil
}
