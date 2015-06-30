// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package signature_test

import (
	"reflect"
	"testing"

	"github.com/luci/luci-go/server/common/auth/signature"
)

func TestX509CertByNameShouldReturnNilIfNotFound(t *testing.T) {
	pc := &signature.PublicCertificates{}
	actual := signature.X509CertByName(pc, "nonexist")
	if actual != nil {
		t.Errorf(`X509CertByName("nonexist")=%v; want <nil>`, actual)
	}
}

func TestX509CertByNameShouldFindIfExist(t *testing.T) {
	key := "dummy"
	expected := []byte("dummy_pem")
	pc := &signature.PublicCertificates{
		Certificates: []signature.Certificate{
			{
				KeyName:            key,
				X509CertificatePEM: string(expected),
			},
		},
	}
	actual := signature.X509CertByName(pc, key)
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf(`X509CertByName(%q)=%q; want %q`, key, actual, expected)
	}
}

func TestX509CertByNameShouldFindFromSeveralCerts(t *testing.T) {
	key := "dummy"
	expected := []byte("dummy_pem")
	pc := &signature.PublicCertificates{
		Certificates: []signature.Certificate{
			{
				KeyName:            "another",
				X509CertificatePEM: "another PEM",
			},
			{
				KeyName:            key,
				X509CertificatePEM: string(expected),
			},
			{
				KeyName:            "the other",
				X509CertificatePEM: "the other PEM",
			},
		},
	}
	actual := signature.X509CertByName(pc, key)
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf(`X509CertByName(%q)=%q; want %q`, key, actual, expected)
	}
}
