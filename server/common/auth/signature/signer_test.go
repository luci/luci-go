// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build !appengine

package signature_test

import (
	"testing"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/common/auth/signature"
)

func TestShouldSignAndCheck(t *testing.T) {
	c := context.Background()
	blob := []byte("blob")
	key, sig, err := signature.Sign(c, blob)
	if err != nil {
		t.Fatalf("Sign(_, %v)=_,_,%v; want <nil>", blob, err)
	}

	pc, err := signature.PublicCerts(c)
	if err != nil {
		t.Fatalf("PublicCerts(_)=%v; want <nil>", err)
	}

	cert := signature.X509CertByName(pc, key)
	if cert == nil {
		t.Fatalf("X509CertByName(%v, %v)=<nil>; want non nil", pc, key)
	}

	err = signature.Check(blob, cert, sig)
	if err != nil {
		t.Errorf("Check(%v, %v, %v)=%v; want <nil>", blob, cert, sig, err)
	}
}
