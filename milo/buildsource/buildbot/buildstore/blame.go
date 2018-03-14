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
	"regexp"
	"strings"

	"github.com/golang/protobuf/ptypes"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/gae/service/memcache"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gitpb "go.chromium.org/luci/common/proto/git"

	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/git"
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
	if len(marshaled) > 1<<20 {
		report(nil, "cannot save > 1MB")
	} else {
		cache.SetValue(marshaled)
		if err := memcache.Set(c, cache); err != nil {
			report(err, "failed to save")
		}
	}
	return nil
}

// fetchChanges populates b.SourceStamp.Changes from Gitiles.
//
// Uses memcache to read the revision of the previous build.
// If not available, loads the previous build that has a commit hash revision.
func fetchChanges(c context.Context, b *buildbot.Build) error {
	memcache.Set(c, buildRevCache(c, b))

	// initialize the slice so that when serialized to JSON, it is [], not null.
	b.Sourcestamp.Changes = []buildbot.Change{}

	host, project, err := gitiles.ParseRepoURL(b.Sourcestamp.Repository)
	if err != nil {
		logging.Warningf(
			c,
			"build %q does not have a valid Gitiles repository URL, %q. Skipping blamelist computation",
			b.ID(), b.Sourcestamp.Repository)
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

	// we don't really need a blamelist with a length > 50
	commits, err := git.Log(c, host, project, b.Sourcestamp.Revision, 50)
	switch status.Code(err) {
	case codes.OK:
		for _, commit := range commits {
			if commit.Id == prevRev {
				break
			}
			change := changeFromGitiles(b.Sourcestamp.Repository, "master", commit)
			b.Sourcestamp.Changes = append(b.Sourcestamp.Changes, change)
		}
		return nil

	case codes.NotFound:
		logging.WithError(err).Warningf(
			c,
			"gitiles.log returned 404 %s/+/%s",
			b.Sourcestamp.Repository, b.Sourcestamp.Revision)
		b.Sourcestamp.Changes = nil
		return nil

	default:
		return err
	}
}

// changeFromGitiles converts a gitiles.Commit to a buildbot change.
func changeFromGitiles(repoURL, branch string, commit *gitpb.Commit) buildbot.Change {
	ct, _ := ptypes.Timestamp(commit.Committer.Time)
	return buildbot.Change{
		At:         ct.Format("Mon _2 Jan 2006 15:04:05"),
		Branch:     &branch,
		Comments:   commit.Message,
		Repository: repoURL,
		Rev:        commit.Id,
		Revision:   commit.Id,
		Revlink:    fmt.Sprintf("%s/+/%s", strings.TrimSuffix(repoURL, "/"), commit.Id),
		When:       int(ct.Unix()),
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
