// Copyright 2024 The LUCI Authors.
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

package acls

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/layered"

	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit"
)

// cacheTTL is the lifetime of the linked accounts cache.
const cacheTTL = 3 * time.Hour

// linkedAccountsCache caches email addresses linked within gerrit.
//
// The cache uses the host and the email address as the key for the cached set
// of emails. The cache also stores a reverse index for each of the email within
// the linked emails set so that the set could be retrieved with any email
// within it.
var linkedAccountsCache = layered.RegisterCache(layered.Parameters[[]string]{
	ProcessCacheCapacity: 0,
	GlobalNamespace:      "gerrit_linked_accounts_cache",
	Marshal: func(emails []string) ([]byte, error) {
		return json.Marshal(emails)
	},
	Unmarshal: func(blob []byte) ([]string, error) {
		out := make([]string, 0, 100)
		err := json.Unmarshal(blob, &out)
		return out, err
	},
})

// honorGerritLinkedAccountsCaches caches whether honor Gerrit linked account
// is enabled for up to 200 LUCI projects.
var honorGerritLinkedAccountsCaches = caching.RegisterLRUCache[string, bool](200)

// linkedAccountKey constructs the cache key for the given host and email.
func linkedAccountKey(host string, email string) string {
	return fmt.Sprintf("linked_account:%s:%s", host, email)
}

// listActiveAccountEmails returns all non-pending confirmation emails linked to the given gerrit account.
func listActiveAccountEmails(ctx context.Context, gf gerrit.Factory, gerritHost string, luciProject string, email string) ([]string, error) {
	client, err := gf.MakeClient(ctx, gerritHost, luciProject)
	if err != nil {
		return nil, err
	}
	res, err := client.ListAccountEmails(ctx, &gerritpb.ListAccountEmailsRequest{Email: email})
	if err != nil {
		return nil, err
	}

	var emails []string
	for _, email := range res.GetEmails() {
		// Ignore emails that are pending confirmation since these emails are used for ACL checks.
		if !email.GetPendingConfirmation() {
			emails = append(emails, email.GetEmail())
		}
	}

	return emails, nil
}

// cacheAllEmails indexes each email within emails to cache.
//
// Used to force-index all the linked emails for a given gerrit account as keys.
// This allows looking up any linked email addresses using any of the email
// within the set. Indexes all the emails (except for ignoreEmail) into cache.
func cacheAllEmails(ctx context.Context, gerritHost string, ignoreEmail string, emails []string) error {
	for _, email := range emails {
		if email == ignoreEmail {
			continue
		}
		_, err := linkedAccountsCache.GetOrCreate(ctx, linkedAccountKey(gerritHost, email), func() (v []string, exp time.Duration, err error) {
			return emails, cacheTTL, err
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func shouldHonorGerritLinkedAccounts(ctx context.Context, project string) (bool, error) {
	cache := honorGerritLinkedAccountsCaches.LRU(ctx)
	if cache == nil {
		return false, errors.New("process cache data is missing")
	}
	return cache.GetOrCreate(ctx, project, func() (v bool, exp time.Duration, err error) {
		meta, err := prjcfg.GetLatestMeta(ctx, project)
		if err != nil {
			return false, 0, err
		}
		if len(meta.ConfigGroupIDs) == 0 {
			return false, 0, errors.Reason("project %q doesn't have any config group", project).Err()
		}
		// honor_gerrit_linked_accounts is a project level field so it will be the
		// same for all config group. Pick the first config group here.
		cg, err := prjcfg.GetConfigGroup(ctx, project, meta.ConfigGroupIDs[0])
		if err != nil {
			return false, 0, err
		}
		return cg.HonorGerritLinkedAccounts, 10 * time.Minute, nil
	})
}

// IsMember checks whether the given identity is a member of any given groups.
//
// If the LUCI project is configured to honor Gerrit linked accounts, in
// addition to checking whether the given identity belongs to the group, this
// function will also return true if any of the linked accounts in the provided
// gerrit host is a member of provided groups.
func IsMember(ctx context.Context, gf gerrit.Factory, gerritHost string, luciProject string, id identity.Identity, groups []string) (bool, error) {
	switch yes, err := auth.GetState(ctx).DB().IsMember(ctx, id, groups); {
	case err != nil:
		return false, err
	case yes:
		return true, nil
	}

	switch yes, err := shouldHonorGerritLinkedAccounts(ctx, luciProject); {
	case err != nil:
	case !yes:
		return false, nil
	}

	var cacheMissed bool
	idEmail := id.Email()
	emails, err := linkedAccountsCache.GetOrCreate(ctx, linkedAccountKey(gerritHost, idEmail), func() (v []string, exp time.Duration, err error) {
		emails, err := listActiveAccountEmails(ctx, gf, gerritHost, luciProject, idEmail)
		cacheMissed = true
		return emails, cacheTTL, err
	})
	if err != nil {
		logging.Errorf(ctx, "Unable to get account information, unable to check linked accounts: %v", err)
		return false, nil
	} else if cacheMissed {
		// Index all of the linked email addresses on a cache miss for the given
		// account.
		if err := cacheAllEmails(ctx, gerritHost, idEmail, emails); err != nil {
			return false, nil
		}
	}

	// Check authorization for all linked email accounts
	for _, email := range emails {
		// Skip already checked.
		if email == idEmail {
			continue
		}
		emailIdentity, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, email))
		if err != nil {
			// Skip malformed identities, e.g. "*@foo.com".
			logging.Warningf(ctx, "Malformed user identity %s: %v", email, err)
			continue
		}

		switch yes, err := auth.GetState(ctx).DB().IsMember(ctx, emailIdentity, groups); {
		case err != nil:
			return false, errors.Annotate(err, "auth.IsMember").Err()
		case yes:
			return true, nil
		}
	}
	return false, nil
}
