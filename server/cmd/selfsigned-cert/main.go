// Copyright 2023 The LUCI Authors.
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

// Executable selfsigned-cert generates a self-signed TLS certificate.
//
// Largely based on https://go.dev/src/crypto/tls/generate_cert.go
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

var (
	hostname = flag.String("hostname", "", "Server hostname (will be used as Subject CN), default to OS hostname")
	duration = flag.Duration("duration", 5*365*24*time.Hour, "Duration that certificate is valid for")
	rsaBits  = flag.Int("rsa-bits", 4096, "Size of RSA key to generate")
	keyOut   = flag.String("key-out", "key.pem", "Where to store the private key")
	certOut  = flag.String("cert-out", "cert.pem", "Where to store the certificate")
)

func main() {
	flag.Parse()
	if *hostname == "" {
		var err error
		if *hostname, err = os.Hostname(); err != nil {
			log.Fatalf("Failed to get hostname: %s", err)
		}
	}

	log.Printf("Generating %d bit RSA key", *rsaBits)
	priv, err := rsa.GenerateKey(rand.Reader, *rsaBits)
	if err != nil {
		log.Fatalf("Failed to generate RSA key: %s", err)
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		log.Fatalf("Unable to marshal private key: %s", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		log.Fatalf("Failed to generate serial number: %s", err)
	}

	log.Printf("Generating certificate for %q, SN %s", *hostname, serialNumber)
	template := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{CommonName: *hostname},
		DNSNames:              []string{*hostname},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(*duration),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		log.Fatalf("Failed to create certificate: %s", err)
	}

	writePEM(*certOut, "CERTIFICATE", certBytes)
	writePEM(*keyOut, "PRIVATE KEY", privBytes)
}

func writePEM(path, typ string, bytes []byte) {
	log.Printf("Writing %s", path)
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		log.Fatalf("Failed to create path to %s: %s", path, err)
	}
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("Failed to open %s: %s", path, err)
	}
	if err := pem.Encode(f, &pem.Block{Type: typ, Bytes: bytes}); err != nil {
		log.Fatalf("Failed to write data to %s: %s", path, err)
	}
	if err := f.Close(); err != nil {
		log.Fatalf("Error closing %s: %s", path, err)
	}
}
