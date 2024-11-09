// Copyright 2016 The LUCI Authors.
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

// Package certauthorities implements CertificateAuthorities API.
//
// Code defined here is either invoked by an administrator or by the service
// itself (via cron jobs or task queues).
package certauthorities

import (
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/certchecker"
	"go.chromium.org/luci/tokenserver/appengine/impl/certconfig"
)

// serverImpl implements admin.CertificateAuthoritiesServer RPC interface.
type serverImpl struct {
	admin.UnsafeCertificateAuthoritiesServer

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
