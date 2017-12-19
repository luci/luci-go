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
	"compress/gzip"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/service/memcache"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server/auth"

	milo "go.chromium.org/luci/milo/api/proto"
)

var (
	// Metrics
	getHistoryCounter = metric.NewCounter(
		"luci/milo/git/GetHistory/cache",
		"The number of hits we get in git.GetHistory",
		nil,
		field.Bool("hit"),
		field.String("repo"),
		field.String("ref"))
)

var gitHash = regexp.MustCompile("[0-9a-fA-F]{40}")

// Resolve resolves a commitish to a git commit hash.
//
// This operation will assumed to be either fully local (in the case that
// commitish is already a git-hash-looking-thing), or local to the datastore (in
// the case that commitish is a fully-qualified-ref, ref tables are populated by
// a backend cron).
//
// If commitish is some other pattern (e.g. "HEAD~"), this will return the
// commitish as-is.
//
// `resolved` will be true if `commit` is, in fact, a git hash; if it's false,
// then higher layers should be careful about using it as part of a cache key.
func Resolve(c context.Context, url, commitish string) (commit string, resolved bool, err error) {
	if gitHash.MatchString(commitish) {
		return commitish, true, nil
	}

	// TODO(iannucci): actually do lookup and cache it. Maybe have a backend cron
	// which refreshes the entire refs space?
	return commitish, false, nil
}

// protoCache maintains a single memcache entry containing a gzipped
// proto.Message.
//
// Args:
//   - key: the memcache key to read/write
//   - out: the proto.Message which should be populated from the cache. This is
//     always a pointer-to-a-proto-struct.
//   - get: populates `out` 'the slow way' returning a duration to cache the
//     result (<=0 means "infinite") or error if encountered.
//
// Example:
//   useCache := cachingEnabled()
//   obj := &mypb.Object{}  // some protobuf object
//   return obj, protoCache(c, useCache, "memcache_key", func() error {
//     return PopulateFromNetwork(obj)
//   })
func protoCache(c context.Context, key string, out proto.Message, get func() (time.Duration, error)) error {
	cacheEntry := memcache.NewItem(c, key+"|gz")
	// try reading from cache
	ok := func() bool {
		switch err := memcache.Get(c, cacheEntry); err {
		case nil:
			r, err := gzip.NewReader(bytes.NewReader(cacheEntry.Value()))
			if err != nil {
				logging.WithError(err).Warningf(c, "making ungzip reader for memcache entry")
				return false
			}

			data, err := ioutil.ReadAll(r)
			if err != nil {
				logging.WithError(err).Warningf(c, "ungzipping memcache entry")
				return false
			}

			if err := proto.Unmarshal(data, out); err != nil {
				logging.WithError(err).Warningf(c, "unmarshalling cache entry")
				return false
			}

			return true
		case memcache.ErrCacheMiss:
		default:
			logging.WithError(err).Warningf(c, "memcache lookup")
		}
		return false
	}()
	if ok {
		return nil
	}

	expiration, err := get()
	if err != nil {
		return err
	}

	data, err := proto.Marshal(out)
	if err != nil {
		logging.WithError(err).Warningf(c, "marshaling proto")
		return nil
	}

	var buf bytes.Buffer
	wr := gzip.NewWriter(&buf)
	wr.Write(data) // err is buffered on the writer till Close
	if err := wr.Close(); err != nil {
		logging.WithError(err).Warningf(c, "gzipping proto")
		return nil
	}

	cacheEntry.SetValue(buf.Bytes())
	if expiration >= 0 {
		cacheEntry.SetExpiration(expiration)
	}
	if err := memcache.Set(c, cacheEntry); err != nil {
		logging.WithError(err).Warningf(c, "memcache set")
	}

	return nil
}

// GetHistory makes a (cached) call to gitiles to obtain the ConsoleGitInfo for
// the given url, commitish and limit.
func GetHistory(c context.Context, url, commitish string, limit int) (*milo.ConsoleGitInfo, error) {
	commitish, resolved, err := Resolve(c, url, commitish)
	if err != nil {
		return nil, errors.Annotate(err, "resolving %q", commitish).Err()
	}
	var expiration time.Duration
	if resolved {
		// The commitish was resolved to a commit id, so we can cache this for
		// a long time.
		expiration = 12 * time.Hour
	} else {
		// If the commitish wasn't resolved, we still want to cache, but not for
		// as long.
		expiration = 30 * time.Second
	}

	ret := &milo.ConsoleGitInfo{}
	cacheKey := fmt.Sprintf("GetHistory|%s|%s|%d", url, commitish, limit)
	cacheHit := true
	err = protoCache(c, cacheKey, ret, func() (time.Duration, error) {
		cacheHit = false
		t, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(gitiles.OAuthScope))
		if err != nil {
			return 0, errors.Annotate(err, "getting RPC Transport").Err()
		}
		g := &gitiles.Client{Client: &http.Client{Transport: t}, Auth: true}
		rawEntries, err := g.Log(c, url, commitish, gitiles.Limit(limit))
		if err != nil {
			return 0, errors.Annotate(err, "GetHistory").Err()
		}

		ret.Commits = make([]*milo.ConsoleGitInfo_Commit, len(rawEntries))

		for i, e := range rawEntries {
			commit := &milo.ConsoleGitInfo_Commit{}
			if commit.Hash, err = hex.DecodeString(e.Commit); err != nil {
				return 0, errors.Annotate(err, "commit is not hex (%q)", e.Commit).Err()
			}

			commit.AuthorName = e.Author.Name
			commit.AuthorEmail = e.Author.Email

			commit.CommitTime = google.NewTimestamp(e.Committer.Time.Time)
			commit.Msg = e.Message

			ret.Commits[i] = commit
		}
		return expiration, nil
	})

	ref := commitish
	if resolved {
		ref = "PINNED"
	}
	getHistoryCounter.Add(c, 1, cacheHit, url, ref)
	return ret, err
}
