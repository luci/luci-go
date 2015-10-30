// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package signingtest

import (
	"testing"
	"time"

	"github.com/luci/luci-go/server/auth/signing"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSigner(t *testing.T) {
	Convey("Works", t, func() {
		ctx := context.Background()

		// Certificates are deterministically built from the seed.
		s := NewSigner(0)
		certs, err := s.Certificates(ctx)
		So(err, ShouldBeNil)
		So(certs, ShouldResemble, &signing.PublicCertificates{
			Certificates: []signing.Certificate{
				{
					KeyName: "fd77904f8cb78191676471b15d05a6508b606ed7",
					X509CertificatePEM: "-----BEGIN CERTIFICATE-----\n" +
						"MIIBDjCBu6ADAgECAgEBMAsGCSqGSIb3DQEBCzAAMCAXDTAxMDkwOTAxNDY0MFoY\n" +
						"DzIyODYxMTIwMTc0NjQwWjAAMFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBAMGYtc/k\n" +
						"vp1Sr2zZFWPu534tqX9chKxhADlLbPR4A+ojKl/EchYCV6DE7Ikogx02PFpYZe3A\n" +
						"3a4hccSufwr3wtMCAwEAAaMgMB4wDgYDVR0PAQH/BAQDAgCAMAwGA1UdEwEB/wQC\n" +
						"MAAwCwYJKoZIhvcNAQELA0EAI/3v5eWNzA2oudenR8Vo5EY0j3zCUVhlHRErlcUR\n" +
						"I69yAHZUpJ9lzcwmHcaCJ76m/jDINZrYoL/4aSlDEGgHmw==\n" +
						"-----END CERTIFICATE-----\n",
				},
			},
			Timestamp: signing.JSONTime(time.Unix(1000000000, 0)),
		})

		// Signatures are also deterministic.
		key, sig, err := s.SignBytes(ctx, []byte("some blob"))
		So(err, ShouldBeNil)
		So(key, ShouldEqual, "fd77904f8cb78191676471b15d05a6508b606ed7")
		So(sig, ShouldResemble, []byte{
			0x66, 0x2d, 0xa6, 0xa0, 0x65, 0x63, 0x8b, 0x83, 0xc5, 0x45, 0xeb, 0xfd,
			0x88, 0xec, 0x9, 0x41, 0x59, 0x92, 0xd0, 0x48, 0x78, 0x37, 0xc2, 0x45,
			0x74, 0xfc, 0x8b, 0x13, 0xa, 0xca, 0x47, 0x7d, 0xd1, 0x24, 0x2c, 0x6c,
			0xbe, 0x3a, 0xea, 0xc5, 0x12, 0x76, 0xb4, 0xe1, 0xa9, 0x4a, 0x40, 0x40,
			0x24, 0xf7, 0x1e, 0x7c, 0x91, 0x91, 0xe3, 0x71, 0x4f, 0x21, 0xf4, 0xe4,
			0xec, 0x65, 0x87, 0x1c,
		})
	})
}
