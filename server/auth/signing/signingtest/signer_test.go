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
	"crypto/x509"
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSigner(t *testing.T) {
	Convey("Works", t, func() {
		ctx := context.Background()

		s := NewSigner(nil)
		certs, err := s.Certificates(ctx)
		So(err, ShouldBeNil)
		So(certs.Certificates, ShouldHaveLength, 1)

		key, sig, err := s.SignBytes(ctx, []byte("some blob"))
		So(err, ShouldBeNil)
		So(key, ShouldEqual, certs.Certificates[0].KeyName)

		// The signature can be verified.
		cert, err := certs.CertificateForKey(key)
		So(err, ShouldBeNil)
		err = cert.CheckSignature(x509.SHA256WithRSA, []byte("some blob"), sig)
		So(err, ShouldBeNil)
	})
}
