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

package signing

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var certBlob = `-----BEGIN CERTIFICATE-----
MIIBDjCBu6ADAgECAgEBMAsGCSqGSIb3DQEBCzAAMCAXDTAxMDkwOTAxNDY0MFoY
DzIyODYxMTIwMTc0NjQwWjAAMFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBAMGYtc/k
vp1Sr2zZFWPu534tqX9chKxhADlLbPR4A+ojKl/EchYCV6DE7Ikogx02PFpYZe3A
3a4hccSufwr3wtMCAwEAAaMgMB4wDgYDVR0PAQH/BAQDAgCAMAwGA1UdEwEB/wQC
MAAwCwYJKoZIhvcNAQELA0EAI/3v5eWNzA2oudenR8Vo5EY0j3zCUVhlHRErlcUR
I69yAHZUpJ9lzcwmHcaCJ76m/jDINZrYoL/4aSlDEGgHmw==
-----END CERTIFICATE-----
`

func TestFetchCertificates(t *testing.T) {
	Convey("Works", t, func() {
		ctx := caching.WithEmptyProcessCache(context.Background())

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(fmt.Sprintf(`{
				"service_account_name": "blah@blah.com",
				"certificates": [{
					"key_name": "abc",
					"x509_certificate_pem": %q
				}],
				"timestamp": 1446166229439210
			}`, certBlob)))
		}))
		certs, err := FetchCertificates(ctx, ts.URL)
		So(err, ShouldBeNil)
		So(certs.ServiceAccountName, ShouldEqual, "blah@blah.com")
		So(len(certs.Certificates), ShouldEqual, 1)
	})

	Convey("Errors", t, func() {
		ctx := caching.WithEmptyProcessCache(context.Background())

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "fail", 401)
		}))
		_, err := FetchCertificates(ctx, ts.URL)
		So(err, ShouldNotBeNil)
	})

	Convey("Bad JSON", t, func() {
		ctx := caching.WithEmptyProcessCache(context.Background())

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(fmt.Sprintf(`{
				"certificates": [{
					"key_name": "abc",
					"x509_certificate_pem": %q
				}],
				"timestamp": "not an int"
			}`, certBlob)))
		}))
		_, err := FetchCertificates(ctx, ts.URL)
		So(err, ShouldNotBeNil)
	})
}

func TestFetchCertificatesForServiceAccount(t *testing.T) {
	Convey("Works", t, func() {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/robot@robots.gserviceaccount.com" {
				http.Error(w, "Wrong URL", 404)
				return
			}
			w.Write([]byte(`{
					"0392f9886770640357cbb29e57d3698291b1e805": "-----BEGIN CERTIFICATE-----\nblah 1\n-----END CERTIFICATE-----\n",
					"f5db308971078d1496c262cc06b6e7f87652af55": "-----BEGIN CERTIFICATE-----\nblah 2\n-----END CERTIFICATE-----\n"
			}`))
		}))

		c := caching.WithEmptyProcessCache(context.Background())
		c = context.WithValue(c, robotCertURLKey(0), ts.URL+"/")
		certs, err := FetchCertificatesForServiceAccount(c, "robot@robots.gserviceaccount.com")
		So(err, ShouldBeNil)
		So(certs.ServiceAccountName, ShouldEqual, "robot@robots.gserviceaccount.com")
		So(certs.Certificates, ShouldResemble, []Certificate{
			{
				KeyName:            "0392f9886770640357cbb29e57d3698291b1e805",
				X509CertificatePEM: "-----BEGIN CERTIFICATE-----\nblah 1\n-----END CERTIFICATE-----\n",
			},
			{
				KeyName:            "f5db308971078d1496c262cc06b6e7f87652af55",
				X509CertificatePEM: "-----BEGIN CERTIFICATE-----\nblah 2\n-----END CERTIFICATE-----\n",
			},
		})
	})
}

func TestCertificateForKey(t *testing.T) {
	Convey("Works", t, func() {
		certs := PublicCertificates{
			Certificates: []Certificate{
				{
					KeyName:            "abc",
					X509CertificatePEM: certBlob,
				},
			},
		}
		cert, err := certs.CertificateForKey("abc")
		So(err, ShouldBeNil)
		So(cert, ShouldNotBeNil)

		// Code coverage for cache hit.
		cert, err = certs.CertificateForKey("abc")
		So(err, ShouldBeNil)
		So(cert, ShouldNotBeNil)
	})

	Convey("Bad PEM", t, func() {
		certs := PublicCertificates{
			Certificates: []Certificate{
				{
					KeyName:            "abc",
					X509CertificatePEM: "not a pem",
				},
			},
		}
		cert, err := certs.CertificateForKey("abc")
		So(err, ShouldErrLike, "not PEM")
		So(cert, ShouldBeNil)
	})

	Convey("Bad cert", t, func() {
		certs := PublicCertificates{
			Certificates: []Certificate{
				{
					KeyName: "abc",
					X509CertificatePEM: string(pem.EncodeToMemory(&pem.Block{
						Type:  "CERTIFICATE",
						Bytes: []byte("not a certificate"),
					})),
				},
			},
		}
		cert, err := certs.CertificateForKey("abc")
		So(err, ShouldErrLike, "structure error")
		So(cert, ShouldBeNil)
	})

	Convey("Missing key", t, func() {
		certs := PublicCertificates{}
		cert, err := certs.CertificateForKey("abc")
		So(err, ShouldErrLike, "no such certificate")
		So(cert, ShouldBeNil)
	})
}

func TestCheckSignature(t *testing.T) {
	// See signingtest/signer_test.go for where this cert and signature were
	// generated. 'signingtest' module itself can't be imported due to import
	// cycle.

	Convey("Works", t, func() {
		certs := PublicCertificates{
			Certificates: []Certificate{
				{
					KeyName:            "abc",
					X509CertificatePEM: certBlob,
				},
			},
		}

		blob := []byte("some blob")

		signature := []byte{
			0x66, 0x2d, 0xa6, 0xa0, 0x65, 0x63, 0x8b, 0x83, 0xc5, 0x45, 0xeb, 0xfd,
			0x88, 0xec, 0x9, 0x41, 0x59, 0x92, 0xd0, 0x48, 0x78, 0x37, 0xc2, 0x45,
			0x74, 0xfc, 0x8b, 0x13, 0xa, 0xca, 0x47, 0x7d, 0xd1, 0x24, 0x2c, 0x6c,
			0xbe, 0x3a, 0xea, 0xc5, 0x12, 0x76, 0xb4, 0xe1, 0xa9, 0x4a, 0x40, 0x40,
			0x24, 0xf7, 0x1e, 0x7c, 0x91, 0x91, 0xe3, 0x71, 0x4f, 0x21, 0xf4, 0xe4,
			0xec, 0x65, 0x87, 0x1c,
		}

		err := certs.CheckSignature("abc", blob, signature)
		So(err, ShouldBeNil)

		err = certs.CheckSignature("abc", blob, []byte{1, 2, 3})
		So(err, ShouldNotBeNil)

		err = certs.CheckSignature("no key", blob, signature)
		So(err, ShouldNotBeNil)
	})
}
