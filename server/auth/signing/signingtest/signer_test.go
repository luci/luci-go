// Copyright 2015 The LUCI Authors.
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

package signingtest

import (
	"context"
	"crypto/x509"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSigner(t *testing.T) {
	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx := context.Background()

		s := NewSigner(nil)
		certs, err := s.Certificates(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, certs.Certificates, should.HaveLength(1))

		key, sig, err := s.SignBytes(ctx, []byte("some blob"))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, key, should.Equal(certs.Certificates[0].KeyName))

		// The signature can be verified.
		cert, err := certs.CertificateForKey(key)
		assert.Loosely(t, err, should.BeNil)
		err = cert.CheckSignature(x509.SHA256WithRSA, []byte("some blob"), sig)
		assert.Loosely(t, err, should.BeNil)
	})
}
