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

package certchecker

import (
	"crypto/x509"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
	"github.com/luci/luci-go/tokenserver/appengine/impl/utils"
)

// CheckCertificateRPC implements CertificateAuthorities.CheckCertificate
// RPC method.
type CheckCertificateRPC struct {
}

// CheckCertificate says whether a certificate is valid or not.
func (r *CheckCertificateRPC) CheckCertificate(c context.Context, req *admin.CheckCertificateRequest) (*admin.CheckCertificateResponse, error) {
	// Deserialize the cert.
	der, err := utils.ParsePEM(req.CertPem, "CERTIFICATE")
	if err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "can't parse 'cert_pem' - %s", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "can't parse 'cert-pem' - %s", err)
	}

	// Find a checker for the CA that signed the cert, check the certificate.
	checker, err := GetCertChecker(c, cert.Issuer.CommonName)
	if err == nil {
		_, err = checker.CheckCertificate(c, cert)
		if err == nil {
			return &admin.CheckCertificateResponse{
				IsValid: true,
			}, nil
		}
	}

	// Recognize error codes related to CA cert checking. Everything else is
	// transient errors.
	if details, ok := err.(Error); ok {
		return &admin.CheckCertificateResponse{
			IsValid:       false,
			InvalidReason: details.Error(),
		}, nil
	}
	return nil, grpc.Errorf(codes.Internal, "failed to check the certificate - %s", err)
}
