// Copyright 2018 The LUCI Authors.
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

package notify

import (
	"context"

	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/mailtmpl"
)

// bundle is a wrapper around mailtmpl.Bundle to provide extra info
// relevant only on server.
type bundle struct {
	*mailtmpl.Bundle
	revision string
}

// bundleCache is a in-process cache of email template bundles.
var bundleCache = caching.RegisterLRUCache[string, *bundle](128)

// getBundle returns a bundle of all email templates for the given project.
// The returned bundle is cached in the process memory, do not modify it.
//
// Returns an error only on transient failures.
//
// Ignores an existing Datastore transaction in c, if any.
func getBundle(c context.Context, projectID string) (*bundle, error) {
	// Untie c from the current transaction.
	// What we do here has nothing to do with a possible current transaction in c.
	c = datastore.WithoutTransaction(c)

	// Fetch current revision of the project config.
	project := &config.Project{Name: projectID}
	if err := datastore.Get(c, project); err != nil {
		return nil, errors.Annotate(err, "failed to fetch project").Err()
	}

	// Lookup an existing bundle in the process cache.
	// If not available, make one and cache it.
	var transientErr error
	value, ok := bundleCache.LRU(c).Mutate(c, projectID, func(it *lru.Item[*bundle]) *lru.Item[*bundle] {
		if it != nil && it.Value.revision == project.Revision {
			return it // Cache hit.
		}

		// Cache miss. Either no cached value or revision mismatch.

		// Fetch all templates from the Datastore transactionally with the project.
		// On a transient error, return it and do not purge cache.
		var templateEntities []*config.EmailTemplate
		transientErr = datastore.RunInTransaction(c, func(c context.Context) error {
			templateEntities = templateEntities[:0] // txn may be retried
			if err := datastore.Get(c, project); err != nil {
				return err
			}

			q := datastore.NewQuery("EmailTemplate").Ancestor(datastore.KeyForObj(c, project))
			return datastore.GetAll(c, q, &templateEntities)
		}, nil)
		if transientErr != nil {
			return it
		}
		logging.Infof(c, "bundleCache: fetched %d email templates of project %q", len(templateEntities), projectID)

		templates := make([]*mailtmpl.Template, len(templateEntities))
		for i, t := range templateEntities {
			templates[i] = t.Template()
		}

		// Bundle all fetched templates. If bundling/parsing fails, cache the error,
		// so we don't recompile bad templates over and over.
		b := &bundle{
			revision: project.Revision,
			Bundle:   mailtmpl.NewBundle(templates),
		}

		// Cache without expiration.
		return &lru.Item[*bundle]{Value: b}
	})

	switch {
	case transientErr != nil:
		return nil, transientErr
	case !ok:
		panic("impossible: no cached value and no error")
	default:
		return value, nil
	}
}
