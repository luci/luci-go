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
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
)

// List of OAuth scopes to use for token sent to CRL endpoint if config doesn't
// specify 'oauth_scopes' field.
var crlFetchDefaultScopes = []string{
	"https://www.googleapis.com/auth/userinfo.email",
}

// FetchCRLRPC implements CertificateAuthorities.FetchCRL RPC method.
type FetchCRLRPC struct {
}

// FetchCRL makes the server fetch a CRL for some CA if it is configured.
func (r *FetchCRLRPC) FetchCRL(c context.Context, req *admin.FetchCRLRequest) (*admin.FetchCRLResponse, error) {
	// Grab a corresponding CA entity. It contains URL of CRL to fetch.
	ca := &CA{CN: req.Cn}
	switch err := ds.Get(c, ca); {
	case err == ds.ErrNoSuchEntity:
		return nil, status.Errorf(codes.NotFound, "no such CA %q", ca.CN)
	case err != nil:
		return nil, status.Errorf(codes.Internal, "datastore error - %s", err)
	}

	// Grab CRL URL from the CA config.
	cfg, err := ca.ParseConfig()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "broken CA config in the datastore - %s", err)
	}

	// No CRL is fine, just don't do anything. The empty response indicates
	// there's no CRL.
	if cfg.CrlUrl == "" {
		return &admin.FetchCRLResponse{}, nil
	}

	// Grab info about last processed CRL, if any.
	crl := &CRL{Parent: ds.KeyForObj(c, ca)}
	if err = ds.Get(c, crl); err != nil && err != ds.ErrNoSuchEntity {
		return nil, status.Errorf(codes.Internal, "datastore error - %s", err)
	}

	// Fetch latest CRL blob.
	logging.Infof(c, "Fetching CRL for %q from %s", ca.CN, cfg.CrlUrl)
	knownETag := crl.LastFetchETag
	if req.Force {
		knownETag = ""
	}
	fetchCtx, cancel := clock.WithTimeout(c, time.Minute)
	defer cancel()
	crlDer, newEtag, err := fetchCRL(fetchCtx, cfg, knownETag)
	switch {
	case transient.Tag.In(err):
		return nil, status.Errorf(codes.Internal, "transient error when fetching CRL - %s", err)
	case err != nil:
		return nil, status.Errorf(codes.Unknown, "can't fetch CRL - %s", err)
	}

	// No changes?
	if knownETag != "" && knownETag == newEtag {
		logging.Infof(c, "No changes to CRL (etag is %s), skipping", knownETag)
	} else {
		logging.Infof(c, "Fetched CRL size is %d bytes, etag is %s", len(crlDer), newEtag)
		crl, err = validateAndStoreCRL(c, crlDer, newEtag, ca, crl)
		switch {
		case transient.Tag.In(err):
			return nil, status.Errorf(codes.Internal, "transient error when storing CRL - %s", err)
		case err != nil:
			return nil, status.Errorf(codes.Unknown, "bad CRL - %s", err)
		}
	}

	return &admin.FetchCRLResponse{CrlStatus: crl.GetStatusProto()}, nil
}

////////////////////////////////////////////////////////////////////////////////

// fetchCRL fetches a blob with pem or der-encoded CRL from the CRL endpoint.
//
// It knows how to use ETag headers to avoid fetching already known data.
// May return transient and fatal errors.
//
// It attempts to parse the fetched data as PEM first. If successful, returns
// decoded DER block. If the data doesn't look like a PEM block, returns it as
// is, assuming it is already in the DER form.
func fetchCRL(c context.Context, cfg *admin.CertificateAuthorityConfig, knownETag string) (blob []byte, etag string, err error) {
	// Pick auth or non-auth transport.
	var transport http.RoundTripper
	if cfg.UseOauth || len(cfg.OauthScopes) != 0 {
		var scopes []string
		if len(cfg.OauthScopes) != 0 {
			scopes = cfg.OauthScopes
		} else {
			scopes = crlFetchDefaultScopes
		}
		transport, err = auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(scopes...))
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

	// Read the body in its entirety (plus new etag, if any).
	blob, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", transient.Tag.Apply(err)
	}
	etag = resp.Header.Get("ETag")

	// Transient error?
	if resp.StatusCode >= http.StatusInternalServerError {
		logging.Warningf(c, "GET %s - HTTP %d; %q", cfg.CrlUrl, resp.StatusCode, string(blob))
		return nil, "",
			transient.Tag.Apply(errors.
				Fmt("server replied with HTTP %d", resp.StatusCode))
	}

	// Something we don't support or expect?
	if resp.StatusCode != http.StatusOK {
		logging.Errorf(c, "GET %s - HTTP %d; %q", cfg.CrlUrl, resp.StatusCode, string(blob))
		return nil, "", fmt.Errorf("unexpected status HTTP %d", resp.StatusCode)
	}

	// Attempt to parse PEM. It's fine if it fails (block == nil), in that case we
	// assume the body is already in the DER form. If it's not really DER, it will
	// fail the subsequent validation step.
	block, rest := pem.Decode(blob)
	if block == nil {
		return blob, etag, nil
	}

	// It is a PEM, but maybe it's not a CRL, or it has multiple blocks. Reject
	// these with fatal errors.
	if block.Type != "X509 CRL" {
		logging.Errorf(c, "GET %s - bad CRL, expecting %q PEM, got %q", cfg.CrlUrl, "X509 CRL", block.Type)
		return nil, "", fmt.Errorf("got %q PEM, not a CRL PEM", block.Type)
	}
	if len(rest) != 0 {
		logging.Errorf(c, "GET %s - bad CRL, more than one PEM block", cfg.CrlUrl)
		return nil, "", fmt.Errorf("bad CRL, more than one PEM block")
	}

	// Have a valid X509 CRL PEM, return its decoded body.
	return block.Bytes, etag, nil
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
		toPut := []any{updated}

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
