// Copyright 2015 The LUCI Authors.
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

// Package gaesigner implements signing.Signer interface using GAE App Identity
// API.
package gaesigner

import (
	"context"
	"runtime"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/caching"
)

// Signer implements signing.Signer using GAE App Identity API.
//
// Deprecated: use GetSigner from go.chromium.org/luci/server/auth instead.
type Signer struct{}

// SignBytes signs the blob with some active private key.
//
// Returns the signature and name of the key used.
func (Signer) SignBytes(ctx context.Context, blob []byte) (keyName string, signature []byte, err error) {
	return info.SignBytes(ctx, blob)
}

// Certificates returns a bundle with public certificates for all active keys.
func (Signer) Certificates(ctx context.Context) (*signing.PublicCertificates, error) {
	return getCachedCerts(ctx)
}

// ServiceInfo returns information about the current service.
//
// It includes app ID and the service account name (that ultimately owns the
// signing private key).
func (Signer) ServiceInfo(ctx context.Context) (*signing.ServiceInfo, error) {
	return getCachedInfo(ctx)
}

////

var (
	certCache = caching.RegisterCacheSlot()
	infoCache = caching.RegisterCacheSlot()
)

// cachedCerts caches this app certs in local memory for 1 hour.
func getCachedCerts(ctx context.Context) (*signing.PublicCertificates, error) {
	v, err := certCache.Fetch(ctx, func(any) (any, time.Duration, error) {
		aeCerts, err := info.PublicCertificates(ctx)
		if err != nil {
			return nil, 0, err
		}
		certs := make([]signing.Certificate, len(aeCerts))
		for i, ac := range aeCerts {
			certs[i] = signing.Certificate{
				KeyName:            ac.KeyName,
				X509CertificatePEM: string(ac.Data),
			}
		}
		inf, err := getCachedInfo(ctx)
		if err != nil {
			return nil, 0, err
		}
		return &signing.PublicCertificates{
			AppID:              inf.AppID,
			ServiceAccountName: inf.ServiceAccountName,
			Certificates:       certs,
			Timestamp:          signing.JSONTime(clock.Now(ctx)),
		}, time.Hour, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*signing.PublicCertificates), nil
}

// getCachedINfo caches this app service info in local memory forever.
//
// This info is static during lifetime of the process.
func getCachedInfo(ctx context.Context) (*signing.ServiceInfo, error) {
	v, err := infoCache.Fetch(ctx, func(any) (any, time.Duration, error) {
		account, err := info.ServiceAccount(ctx)
		if err != nil {
			return nil, 0, err
		}
		return &signing.ServiceInfo{
			AppID:              info.AppID(ctx),
			AppRuntime:         "go",
			AppRuntimeVersion:  runtime.Version(),
			AppVersion:         strings.Split(info.VersionID(ctx), ".")[0],
			ServiceAccountName: account,
		}, 0, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*signing.ServiceInfo), nil
}
