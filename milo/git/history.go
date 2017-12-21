// Copyright 2017 The LUCI Authors.
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

package git

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/gae/service/memcache"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server/auth"
)

var getHistoryCounter = metric.NewCounter(
	"luci/milo/git/log/cache",
	"The number of hits we get in git.log",
	nil,
	field.String("result"), // see usage for possible values
	field.String("repo"),
	field.String("ref"))

var gitHash = regexp.MustCompile("^[0-9a-fA-F]{40}$")

// Log makes a (cached) call to gitiles to obtain up to 100 commits for
// the given repo url and commitish..
func Log(c context.Context, repoURL, commitish string) ([]gitiles.Commit, error) {
	c = logging.SetFields(c, logging.Fields{
		"repoURL":   repoURL,
		"commitish": commitish,
	})
	c, err := info.Namespace(c, "git-log")
	if err != nil {
		return nil, errors.Annotate(err, "could not set namespace").Err()
	}

	mkCache := func(commitish string) memcache.Item {
		// do not include limit in the cache key.
		return memcache.NewItem(c, fmt.Sprintf("%s|%s|%d", repoURL, commitish, gitiles.GobVersion))
	}
	cacheEntry := mkCache(commitish)

	commitIsHash := gitHash.MatchString(commitish)
	if !commitIsHash {
		// committish is not pinned, may move, so set an expiration.
		cacheEntry.SetExpiration(30 * time.Second)
	}

	cacheResult := "unknown"
	defer func() {
		ref := commitish
		if commitIsHash {
			ref = "PINNED"
		}
		getHistoryCounter.Add(c, 1, cacheResult, repoURL, ref)
	}()

	// try reading from cache
	switch err := memcache.Get(c, cacheEntry); {
	case err == memcache.ErrCacheMiss:
		cacheResult = "miss"
		logging.Warningf(c, "cache miss for %q", cacheEntry.Key())
	case err != nil:
		cacheResult = "failure-cache"
		logging.Errorf(c, "cache failure for %q", cacheEntry.Key())
	default:
		var commits []gitiles.Commit
		if err := gob.NewDecoder(bytes.NewReader(cacheEntry.Value())).Decode(&commits); err != nil {
			cacheResult = "failure-decoding"
			logging.WithError(err).Errorf(c, "could not decode cached commits")
		} else {
			cacheResult = "hit"
			return commits, nil
		}
	}

	// cache miss, cache failure or corrupted cache

	t, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(gitiles.OAuthScope))
	if err != nil {
		return nil, errors.Annotate(err, "getting RPC Transport").Err()
	}
	g := &gitiles.Client{Client: &http.Client{Transport: t}, Auth: true}
	commits, err := g.Log(c, repoURL, commitish, gitiles.Limit(100))
	if err != nil {
		return nil, errors.Annotate(err, "Log").Err()
	}

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(commits); err != nil {
		logging.WithError(err).Errorf(c, "failed to encode gitiles commits to gob. Commits:\n%#v", commits)
	} else {
		caches := []memcache.Item{cacheEntry}
		cacheEntry.SetValue(buf.Bytes())

		if !commitIsHash && len(commits) > 0 {
			hashCache := mkCache(commits[0].Commit)
			hashCache.SetValue(buf.Bytes())
			caches = append(caches, hashCache)
		}

		if err := memcache.Set(c, caches...); err != nil {
			logging.WithError(err).Errorf(c, "failed to cache gitiles commits")
		}
	}

	return commits, nil
}
