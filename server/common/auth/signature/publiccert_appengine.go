// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build appengine

package signature

import (
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/memcache"
	"google.golang.org/appengine/urlfetch"
)

// PrimaryPublicCertificates returns primary's PublicCertificates.
func PrimaryPublicCertificates(c context.Context, primaryURL string) (*PublicCertificates, error) {
	cacheKey := fmt.Sprintf("pub_certs:%s", primaryURL)
	var pubCerts []byte
	setCache := false
	item, err := memcache.Get(c, cacheKey)
	if err != nil {
		setCache = true
		if err != memcache.ErrCacheMiss {
			log.Warningf(c, "failed to get cert from cache: %v", err)
		}
		pubCerts, err = downloadCert(urlfetch.Client(c), primaryURL)
		if err != nil {
			log.Errorf(c, "failed to download cert: %v", err)
			return nil, err
		}
	} else {
		pubCerts = item.Value
	}
	pc := &PublicCertificates{}
	if err = json.Unmarshal(pubCerts, pc); err != nil {
		log.Errorf(c, "failed to unmarshal cert: %v %v", string(pubCerts), err)
		return nil, err
	}
	if setCache {
		err = memcache.Set(c, &memcache.Item{
			Key:        cacheKey,
			Value:      pubCerts,
			Expiration: time.Hour,
		})
		if err != nil {
			log.Warningf(c, "failed to set cert to cache: %v", err)
		}
	}
	return pc, nil
}
