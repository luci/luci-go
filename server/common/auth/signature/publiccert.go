// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build !appengine

package signature

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/context"
)

var (
	mu sync.Mutex
	// cache is simple way of caching data.
	cache = make(map[string]cacheEntry)
)

type cacheEntry struct {
	publicCerts *PublicCertificates
	expires     time.Time
}

// PrimaryPublicCertificates returns primary's PublicCertificates.
func PrimaryPublicCertificates(c context.Context, primaryURL string) (*PublicCertificates, error) {
	mu.Lock()
	defer mu.Unlock()
	entry, ok := cache[primaryURL]
	if ok && time.Now().Before(entry.expires) {
		return entry.publicCerts, nil
	}

	jsonCerts, err := downloadCert(http.DefaultClient, primaryURL)
	if err != nil {
		return nil, err
	}
	pc := &PublicCertificates{}
	if err := json.Unmarshal(jsonCerts, pc); err != nil {
		return nil, err
	}
	cache[primaryURL] = cacheEntry{
		publicCerts: pc,
		expires:     time.Now().Add(time.Hour),
	}
	return pc, nil
}
