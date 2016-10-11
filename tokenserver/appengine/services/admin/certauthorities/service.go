// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package certauthorities implements CertificateAuthorities API.
//
// Code defined here is either invoked by an administrator or by the service
// itself (via cron jobs or task queues).
package certauthorities

import (
	"github.com/luci/luci-go/tokenserver/appengine/certchecker"
	"github.com/luci/luci-go/tokenserver/appengine/certconfig"

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
)

// serverImpl implements admin.CertificateAuthoritiesServer RPC interface.
type serverImpl struct {
	certconfig.FetchCRLRPC
	certconfig.ListCAsRPC
	certconfig.GetCAStatusRPC
	certchecker.IsRevokedCertRPC
	certchecker.CheckCertificateRPC
}

// NewServer returns prod CertificateAuthoritiesServer implementation.
//
// It assumes authorization has happened already.
func NewServer() admin.CertificateAuthoritiesServer {
	return &serverImpl{}
}
