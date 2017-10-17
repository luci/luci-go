package buildstore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"go.chromium.org/gae/service/memcache"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/server/auth"
	"regexp"
	"strings"
)

var commitHashRe = regexp.MustCompile(`^[a-f0-9]{40}$`)

// This file computes a static blamelist of a buildbot build.

// blame loads changes from gitiles and computes b.Blame.
func blame(c context.Context, b *buildbot.Build) error {
	err := fetchChangesCached(c, b)
	if err != nil {
		return err
	}
	blame := stringset.New(len(b.Sourcestamp.Changes))
	for _, c := range b.Sourcestamp.Changes {
		if blame.Add(c.Who) {
			b.Blame = append(b.Blame, c.Who)
		}
	}
	return nil
}

func fetchChangesCached(c context.Context, b *buildbot.Build) error {
	log := func(err error, msg string) {
		logging.WithError(err).Errorf(c, "build %d change memcaching: %s", msg)
	}

	cache := memcache.NewItem(c, fmt.Sprintf("buildbot_changes/%s/%s/%d", b.Master, b.Buildername, b.Number))
	if err := memcache.Get(c, cache); err == nil {
		var changes []buildbot.Change
		if err := json.Unmarshal(cache.Value(), &changes); err != nil {
			log(err, "failed to unmarshal memcached changes")
		} else {
			b.Sourcestamp.Changes = changes
			return nil
		}
	} else if err != memcache.ErrCacheMiss {
		log(err, "failed to load")
	}

	err := fetchChanges(c, b)
	if err != nil {
		return err
	}

	marshalled, err := json.Marshal(b.Sourcestamp.Changes)
	if err != nil {
		return err
	}
	cache.SetValue(marshalled)
	memcache.Set(c, cache)
	return nil
}

// fetchChanges populates b.SourceStamp.Changes from Gitiles.
//
// Uses revision cache to read the revision of the previous build.
// If not available, loads the previous build.
func fetchChanges(c context.Context, b *buildbot.Build) error {
	memcache.Set(c, buildRevCache(c, b))
	prevRev, err := getPrevRev(c, b)
	switch {
	case err != nil:
		return errors.Annotate(err, "failed to get prev revision for build %q", b.IDString()).Err()
	case prevRev == "":
		return nil
	}

	client := gitiles.Client{
		Client: &http.Client{},
	}
	if strings.HasPrefix(b.Sourcestamp.Repository, "https://chromium.googlesource.com/") {
		client.Client.Transport, err = auth.GetRPCTransport(c, auth.AsSelf)
	} else {
		client.Client.Transport, err = auth.GetRPCTransport(c, auth.NoAuth)
	}
	if err != nil {
		return err
	}

	revRange := prevRev + ".." + b.Sourcestamp.Revision
	commits, err := client.Log(c, b.Sourcestamp.Repository, revRange, gitiles.WithTreeDiff)
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
		branch := "master"
		change := buildbot.Change{
			At:         commit.Committer.Time.Format("Mon_2 Jan 2006 15:04:05"),
			Branch:     &branch,
			Comments:   commit.Message,
			Repository: b.Sourcestamp.Repository,
			Rev:        commit.Commit,
			Revision:   commit.Commit,
			Revlink: fmt.Sprintf(
				"%s/+/%s",
				strings.TrimSuffix(b.Sourcestamp.Repository, "/"),
				commit.Commit),
			When: int(commit.Committer.Time.Unix()),
			Who:  commit.Committer.Email,
		}
		for _, f := range commit.TreeDiff {
			change.Files = append(change.Files, f.NewPath)
		}
		b.Sourcestamp.Changes[i] = change
	}
	return nil
}

// getPrevRev returns revision of the previous build.
// Searches for the previous build until one is found.
// Memcaches results.
func getPrevRev(c context.Context, b *buildbot.Build) (string, error) {
	prev := &buildbot.Build{
		Master:      b.Master,
		Buildername: b.Buildername,
		Number:      b.Number - 1,
	}

	cache := buildRevCache(c, prev)
	err := memcache.Get(c, cache)
	if err == nil {
		return string(cache.Value()), nil
	}

	if err != memcache.ErrCacheMiss {
		logging.WithError(err).Warningf(c, "memcache.get failed for key %q", cache.Key())
	}
	fetched, err := getBuild(c, prev.Master, prev.Buildername, prev.Number, false)
	if err != nil {
		return "", err
	}
	var prevRev string
	if fetched != nil && commitHashRe.MatchString(fetched.Sourcestamp.Revision) {
		prevRev = fetched.Sourcestamp.Revision
	} else {
		if prevRev, err = getPrevRev(c, b); err != nil {
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
	item := memcache.NewItem(c, fmt.Sprintf("buildbot_revision/%s/%s/%d", b.Master, b.Buildername, b.Number))
	item.SetValue([]byte(b.Sourcestamp.Revision))
	return item
}
