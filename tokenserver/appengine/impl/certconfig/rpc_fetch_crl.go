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

package certconfig

import (
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry/transient"
	"github.com/luci/luci-go/server/auth"

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
)

// List of OAuth scopes to use for token sent to CRL endpoint.
var crlFetchScopes = []string{
	"https://www.googleapis.com/auth/userinfo.email",
}

// FetchCRLRPC implements CertificateAuthorities.FetchCRL RPC method.
type FetchCRLRPC struct {
}

// FetchCRL makes the server fetch a CRL for some CA.
func (r *FetchCRLRPC) FetchCRL(c context.Context, req *admin.FetchCRLRequest) (*admin.FetchCRLResponse, error) {
	// Grab a corresponding CA entity. It contains URL of CRL to fetch.
	ca := &CA{CN: req.Cn}
	switch err := ds.Get(c, ca); {
	case err == ds.ErrNoSuchEntity:
		return nil, grpc.Errorf(codes.NotFound, "no such CA %q", ca.CN)
	case err != nil:
		return nil, grpc.Errorf(codes.Internal, "datastore error - %s", err)
	}

	// Grab CRL URL from the CA config.
	cfg, err := ca.ParseConfig()
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "broken CA config in the datastore - %s", err)
	}
	if cfg.CrlUrl == "" {
		return nil, grpc.Errorf(codes.NotFound, "CA %q doesn't have CRL defined", ca.CN)
	}

	// Grab info about last processed CRL, if any.
	crl := &CRL{Parent: ds.KeyForObj(c, ca)}
	if err = ds.Get(c, crl); err != nil && err != ds.ErrNoSuchEntity {
		return nil, grpc.Errorf(codes.Internal, "datastore error - %s", err)
	}

	// Fetch latest CRL blob.
	logging.Infof(c, "Fetching CRL for %q from %s", ca.CN, cfg.CrlUrl)
	knownETag := crl.LastFetchETag
	if req.Force {
		knownETag = ""
	}
	fetchCtx, _ := clock.WithTimeout(c, time.Minute)
	crlDer, newEtag, err := fetchCRL(fetchCtx, cfg, knownETag)
	switch {
	case transient.Tag.In(err):
		return nil, grpc.Errorf(codes.Internal, "transient error when fetching CRL - %s", err)
	case err != nil:
		return nil, grpc.Errorf(codes.Unknown, "can't fetch CRL - %s", err)
	}

	// No changes?
	if knownETag != "" && knownETag == newEtag {
		logging.Infof(c, "No changes to CRL (etag is %s), skipping", knownETag)
	} else {
		logging.Infof(c, "Fetched CRL size is %d bytes, etag is %s", len(crlDer), newEtag)
		crl, err = validateAndStoreCRL(c, crlDer, newEtag, ca, crl)
		switch {
		case transient.Tag.In(err):
			return nil, grpc.Errorf(codes.Internal, "transient error when storing CRL - %s", err)
		case err != nil:
			return nil, grpc.Errorf(codes.Unknown, "bad CRL - %s", err)
		}
	}

	return &admin.FetchCRLResponse{CrlStatus: crl.GetStatusProto()}, nil
}

////////////////////////////////////////////////////////////////////////////////

// fetchCRL fetches a blob with der-encoded CRL from the CRL endpoint.
//
// It knows how to use ETag headers to avoid fetching already known data.
// May return transient and fatal errors.
func fetchCRL(c context.Context, cfg *admin.CertificateAuthorityConfig, knownETag string) (blob []byte, etag string, err error) {
	// Pick auth or non-auth transport.
	var transport http.RoundTripper
	if cfg.UseOauth {
		transport, err = auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(crlFetchScopes...))
	} else {
		transport, err = auth.GetRPCTransport(c, auth.NoAuth)
	}
	if err != nil {
		return nil, "", err
	}

	// Send the request with ETag related headers.
	req, err := http.NewRequest("GET", cfg.CrlUrl, nil)
	if err != nil {
		return nil, "", err
	}
	if knownETag != "" {
		req.Header.Set("If-None-Match", knownETag)
	}
	cl := http.Client{Transport: transport}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, "", transient.Tag.Apply(err)
	}
	defer resp.Body.Close()

	// Nothing new?
	if resp.StatusCode == http.StatusNotModified {
		if knownETag == "" {
			return nil, "", errors.New("unexpected 304 status, no etag header was sent")
		}
		return nil, knownETag, nil
	}

	// Read the body in its entirety.
	blob, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, "", transient.Tag.Apply(err)
	}

	// Transient error?
	if resp.StatusCode >= http.StatusInternalServerError {
		logging.Warningf(c, "GET %s - HTTP %d; %q", cfg.CrlUrl, resp.StatusCode, string(blob))
		return nil, "", errors.Reason("server replied with HTTP %d", resp.StatusCode).
			Tag(transient.Tag).Err()
	}

	// Something we don't support or expect?
	if resp.StatusCode != http.StatusOK {
		logging.Errorf(c, "GET %s - HTTP %d; %q", cfg.CrlUrl, resp.StatusCode, string(blob))
		return nil, "", fmt.Errorf("unexpected status HTTP %d", resp.StatusCode)
	}

	// Good enough.
	return blob, resp.Header.Get("ETag"), nil
}

// validateAndStoreCRL handles incoming CRL blob fetched by 'fetchCRL'.
func validateAndStoreCRL(c context.Context, crlDer []byte, etag string, ca *CA, prev *CRL) (*CRL, error) {
	// Make sure it is signed by the CA.
	caCert, err := x509.ParseCertificate(ca.Cert)
	if err != nil {
		return nil, fmt.Errorf("cert in the datastore is broken - %s", err)
	}
	crl, err := x509.ParseDERCRL(crlDer)
	if err != nil {
		return nil, fmt.Errorf("not a valid x509 CRL - %s", err)
	}
	if err = caCert.CheckCRLSignature(crl); err != nil {
		return nil, fmt.Errorf("CRL is not signed by the CA - %s", err)
	}

	// The CRL is peachy. Update a sharded set of all revoked certs.
	logging.Infof(c, "CRL last updated %s", crl.TBSCertList.ThisUpdate)
	logging.Infof(c, "Found %d entries in the CRL", len(crl.TBSCertList.RevokedCertificates))
	if err = UpdateCRLSet(c, ca.CN, CRLShardCount, crl); err != nil {
		return nil, err
	}
	logging.Infof(c, "All CRL entries stored")

	// Update the CRL entity. Use EntityVersion to make sure we are not
	// overwriting someone else's changes.
	var updated *CRL
	err = ds.RunInTransaction(c, func(c context.Context) error {
		entity := *prev
		if err := ds.Get(c, &entity); err != nil && err != ds.ErrNoSuchEntity {
			return err
		}
		if entity.EntityVersion != prev.EntityVersion {
			return fmt.Errorf("CRL for %q was updated concurrently while we were working on it", ca.CN)
		}
		entity.EntityVersion++
		entity.LastUpdateTime = crl.TBSCertList.ThisUpdate.UTC()
		entity.LastFetchTime = clock.Now(c).UTC()
		entity.LastFetchETag = etag
		entity.RevokedCertsCount = len(crl.TBSCertList.RevokedCertificates)

		updated = &entity // used outside of this function
		toPut := []interface{}{updated}

		// Mark CA entity as ready for usage.
		curCA := CA{CN: ca.CN}
		switch err := ds.Get(c, &curCA); {
		case err == ds.ErrNoSuchEntity:
			return fmt.Errorf("CA entity for %q is unexpectedly gone", ca.CN)
		case err != nil:
			return err
		}
		if !curCA.Ready {
			logging.Infof(c, "CA %q is ready now", curCA.CN)
			curCA.Ready = true
			toPut = append(toPut, &curCA)
		}
		return ds.Put(c, toPut)
	}, nil)
	if err != nil {
		return nil, transient.Tag.Apply(err)
	}

	logging.Infof(c, "CRL for %q is updated, entity version is %d", ca.CN, updated.EntityVersion)
	return updated, nil
}
