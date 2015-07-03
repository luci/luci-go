// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package replication

import (
	"fmt"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/common/auth/model"
	"github.com/luci/luci-go/server/common/auth/signature"
)

// verifySignature verifies the signature for blob.
func verifySignature(c context.Context, keyName string, blob, sig []byte) error {
	rs := &model.AuthReplicationState{}
	if err := model.GetReplicationState(c, rs); err != nil {
		return err
	}
	certs, err := signature.PrimaryPublicCertificates(c, rs.PrimaryURL)
	if err != nil {
		return err
	}
	pem := signature.X509CertByName(certs, keyName)
	if pem == nil {
		return fmt.Errorf("failed to find cert")
	}
	return signature.Check(blob, pem, sig)
}
