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

package buildstore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/memcache"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/server/auth"
)

// This file computes a static blamelist of a buildbot build via Gitiles RPCs.

var commitHashRe = regexp.MustCompile(`^[a-f0-9]{40}$`)

// blame loads changes from gitiles and computes b.Blame and
// b.SourceStamp.Changes from b.SourceStamp.Repository,
// b.SourceStamp.Revision and builds previous to b.
//
// Memcaches results.
func blame(c context.Context, b *buildbot.Build) error {
	if err := fetchChangesCached(c, b); err != nil {
		return err
	}
	blame := stringset.New(len(b.Sourcestamp.Changes))
	b.Blame = make([]string, 0, len(b.Sourcestamp.Changes))
	for _, c := range b.Sourcestamp.Changes {
		if blame.Add(c.Who) {
			b.Blame = append(b.Blame, c.Who)
		}
	}
	return nil
}

// fetchChangesCached is same as fetchChanges, but with memcaching.
func fetchChangesCached(c context.Context, b *buildbot.Build) error {
	report := func(err error, msg string) {
		logging.WithError(err).Errorf(c, "build %q change memcaching: %s", b.ID(), msg)
	}

	cache := memcache.NewItem(c, "buildbot_changes/"+b.ID())
	if err := memcache.Get(c, cache); err == nil {
		err := json.Unmarshal(cache.Value(), &b.Sourcestamp.Changes)
		if err == nil {
			return nil
		}
		b.Sourcestamp.Changes = nil
		report(err, "failed to unmarshal memcached changes")
	} else if err != memcache.ErrCacheMiss {
		report(err, "failed to load")
	}

	if err := fetchChanges(c, b); err != nil {
		return err
	}

	marshaled, err := json.Marshal(b.Sourcestamp.Changes)
	if err != nil {
		return err
	}
	cache.SetValue(marshaled)
	if err := memcache.Set(c, cache); err != nil {
		report(err, "failed to save")
	}
	return nil
}

// fetchChanges populates b.SourceStamp.Changes from Gitiles.
//
// Uses memcache to read the revision of the previous build.
// If not available, loads the previous build that has a commit hash revision.
func fetchChanges(c context.Context, b *buildbot.Build) error {
	memcache.Set(c, buildRevCache(c, b))

	if b.Sourcestamp.Repository == "" {
		logging.Warningf(c, "build %q has no repository URL. Skipping blamelist computation", b.ID())
		return nil
	}
	if !commitHashRe.MatchString(b.Sourcestamp.Revision) {
		logging.Warningf(
			c,
			"build %q revision %q is not a commit hash. Skipping blamelist computation",
			b.Sourcestamp.Revision, b.ID())
		return nil
	}

	prevRev, err := getPrevRev(c, b, 100)
	switch {
	case err != nil:
		return errors.Annotate(err, "failed to get prev revision for build %q", b).Err()
	case prevRev == "":
		logging.Warningf(c, "prev rev of build %q is unknown. Skipping blamelist computation", b.ID())
		return nil
	}

	// Note that prev build may be coming from buildbot and having commit different
	// from the true previous _LUCI_ build, which may cause blamelist to have
	// extra or missing commits. This matters only for the first build after
	// next build number bump.

	client := gitiles.Client{Client: &http.Client{}}
	rpcAuth := auth.NoAuth
	if strings.HasPrefix(b.Sourcestamp.Repository, "https://chromium.googlesource.com/") {
		// Use authentication only for the known public Gitiles host
		// to avoid small anonymous quota.
		// This is the typical case.
		client.Auth = true
		rpcAuth = auth.AsSelf
	}
	if client.Client.Transport, err = auth.GetRPCTransport(c, rpcAuth, auth.WithScopes(gitiles.OAuthScope)); err != nil {
		return err
	}

	revRange := prevRev + ".." + b.Sourcestamp.Revision
	commits, err := client.Log(c, b.Sourcestamp.Repository, revRange)
	switch {
	case gitiles.HTTPStatus(err) == http.StatusNotFound:
		logging.WithError(err).Warningf(c, "gitiles returned 404 for range %q", revRange)
		b.Sourcestamp.Changes = nil
		return nil
	case err != nil:
		return err
	}

	b.Sourcestamp.Changes = make([]buildbot.Change, len(commits))
	for i, commit := range commits {
		b.Sourcestamp.Changes[i] = changeFromGitiles(b.Sourcestamp.Repository, "master", commit)
	}
	return nil
}

// changeFromGitiles converts a gitiles.Commit to a buildbot change.
func changeFromGitiles(repoURL, branch string, commit gitiles.Commit) buildbot.Change {
	return buildbot.Change{
		// Real-world example of At: Fri 13 Oct 2017 13:42:58
		At:         commit.Committer.Time.Format("Mon _2 Jan 2006 15:04:05"),
		Branch:     &branch,
		Comments:   commit.Message,
		Repository: repoURL,
		Rev:        commit.Commit,
		Revision:   commit.Commit,
		Revlink:    fmt.Sprintf("%s/+/%s", strings.TrimSuffix(repoURL, "/"), commit.Commit),
		When:       int(commit.Committer.Time.Unix()),
		Who:        commit.Author.Email,
		// TODO(nodir): add Files if someone needs them.
	}
}

// getPrevRev returns revision of the closest previous build with a commit
// hash, or "" if not found.
// Memcaches results.
func getPrevRev(c context.Context, b *buildbot.Build, maxRecursionDepth int) (string, error) {
	// note: we cannot use exponential scan here because there may be build
	// number gaps anywhere, for example given build numbers 10 20 30 40,
	// if we check 25 while scanning [20, 40), we don't know if we should continue
	// scanning in [20, 25) or (25, 40).

	switch {
	case b.Number == 0:
		return "", nil
	case maxRecursionDepth <= 0:
		logging.Warningf(c, "reached maximum recursion depth; giving up")
		return "", nil
	}

	prev := &buildbot.Build{
		Master:      b.Master,
		Buildername: b.Buildername,
		Number:      b.Number - 1,
	}

	cache := buildRevCache(c, prev)
	err := memcache.Get(c, cache)
	if err == nil {
		// fast path
		return string(cache.Value()), nil
	}

	// slow path
	if err != memcache.ErrCacheMiss {
		logging.WithError(err).Warningf(c, "memcache.get failed for key %q", cache.Key())
	}
	fetched, err := getBuild(c, prev.Master, prev.Buildername, prev.Number, false, false)
	if err != nil {
		return "", err
	}

	var prevRev string
	if fetched != nil && commitHashRe.MatchString(fetched.Sourcestamp.Revision) {
		prevRev = fetched.Sourcestamp.Revision
	} else {
		// slowest path
		// May happen if there is a gap in build numbers or
		// if someone scheduled a build manually with no or HEAD revision.
		// Rare case.

		// This is a recursive call of itself.
		// The results are memcached along the stack though.
		if prevRev, err = getPrevRev(c, b, maxRecursionDepth-1); err != nil {
			return "", err
		}
	}
	cache.SetValue([]byte(prevRev))
	memcache.Set(c, cache)
	return prevRev, nil
}

// buildRevCache returns a memcache.Item for the build's revision.
// Initializes the value with current revision.
func buildRevCache(c context.Context, b *buildbot.Build) memcache.Item {
	item := memcache.NewItem(c, "buildbot_revision/"+b.ID())
	if b.Sourcestamp != nil {
		item.SetValue([]byte(b.Sourcestamp.Revision))
	}
	return item
}
