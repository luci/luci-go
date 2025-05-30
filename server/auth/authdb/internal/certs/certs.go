// Copyright 2019 The LUCI Authors.
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

// Package certs knows how to fetch certificate bundles of trusted services.
package certs

import (
	"context"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/caching/lazyslot"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/auth/signing"
)

// Bundle is a lazy-loaded cert bundle of some LUCI service.
type Bundle struct {
	ServiceURL string        // root URL of the service to fetch the bundle from
	certs      lazyslot.Slot // stores fetchedBundle with the lazily fetched bundle
}

type fetchedBundle struct {
	id    identity.Identity
	certs *signing.PublicCertificates
}

// GetCerts fetches (perhaps from cache) cert bundles of the service.
//
// Returns the service identity as well.
func (b *Bundle) GetCerts(ctx context.Context) (identity.Identity, *signing.PublicCertificates, error) {
	fetched, err := b.certs.Get(ctx, func(any) (any, time.Duration, error) {
		fetched, err := b.fetch(ctx)
		return fetched, time.Hour, err
	})
	if err != nil {
		return "", nil, err
	}
	fetchedBundle := fetched.(*fetchedBundle)
	return fetchedBundle.id, fetchedBundle.certs, nil
}

// fetch actually fetches the cert bundle.
func (b *Bundle) fetch(ctx context.Context) (*fetchedBundle, error) {
	certs, err := signing.FetchCertificatesFromLUCIService(ctx, b.ServiceURL)
	if err != nil {
		return nil, errors.Fmt("failed to fetch certs from %s: %w", b.ServiceURL, err)
	}
	if certs.ServiceAccountName == "" {
		return nil, errors.Fmt("service %s didn't provide its service account name", b.ServiceURL)
	}
	id, err := identity.MakeIdentity("user:" + certs.ServiceAccountName)
	if err != nil {
		return nil, errors.Fmt("invalid service_account_name %q in certificates bundle from %s", certs.ServiceAccountName, b.ServiceURL)
	}
	return &fetchedBundle{id, certs}, nil
}
