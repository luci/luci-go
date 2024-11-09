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

package git

import (
	"context"
	"encoding/hex"
	"fmt"
	"regexp"
	"time"

	"github.com/gomodule/redigo/redis"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gitpb "go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/redisconn"

	"go.chromium.org/luci/milo/internal/utils"
)

// LogOptions are options for Log function.
type LogOptions struct {
	Limit     int
	WithFiles bool
}

// Log implements Client interface.
func (p *implementation) Log(c context.Context, host, project, commitish string, inputOptions *LogOptions) (commits []*gitpb.Commit, err error) {
	defer func() { err = utils.TagGRPC(c, err) }()

	allowed, err := p.acls.IsAllowed(c, host, project)
	switch {
	case err != nil:
		return
	case !allowed:
		logging.Warningf(c, "%q not allowed for host %q project %q according to git ACL configured in milo's settings", auth.CurrentIdentity(c), host, project)
		err = status.Errorf(codes.NotFound, "not found")
		return
	}

	return p.log(c, host, project, commitish, "", inputOptions)
}

func (p *implementation) log(c context.Context, host, project, commitish, ancestor string, inputOptions *LogOptions) (commits []*gitpb.Commit, err error) {
	var opts LogOptions
	if inputOptions != nil {
		opts = *inputOptions
	}
	if opts.Limit <= 0 {
		opts.Limit = 50
	}

	remaining := opts.Limit // defined as (limit - len(commits))
	// add appends page to commits and updates remaining.
	// If page starts with commits' last commit, page's first commit
	// is skipped.
	add := func(page []*gitpb.Commit) {
		switch {
		case len(commits) == 0:
			commits = page
		case len(page) == 0:
			// Do nothing.
		default:
			if page[0].Id != commits[len(commits)-1].Id {
				panic(fmt.Sprintf(
					"pages are not contiguous; want page start %q, got %q",
					commits[len(commits)-1].Id, page[0].Id))
			}
			commits = append(commits, page[1:]...)
		}
		remaining = opts.Limit - len(commits)
	}

	req := &logReq{
		factory:   p,
		host:      host,
		project:   project,
		commitish: commitish,
		ancestor:  ancestor,
		withFiles: opts.WithFiles,
		min:       100,
	}

	var page []*gitpb.Commit

	// We may need to fetch >100 commits, but one logReq can handle only 100.
	// Call it in a loop.
queryLoop:
	for remaining > 0 {
		required := remaining
		if len(commits) > 0 {
			// +1 because the first returned commit will be discarded.
			required++
		}

		// Don't query excessive commits.
		if req.min > required {
			req.min = required
		}
		page, err = req.call(c)
		switch {
		case err != nil:
			return
		case len(page) < 100:
			// This can happen iff there are no more commits.
			add(page)
			break queryLoop
		case len(page) > 100:
			panic("impossible: logReq.call() returned >100 commits")
		default:
			// There may be more commits.
			// Continue from the last fetched commit.
			req.commitish = page[len(page)-1].Id
			add(page)
		}
	}

	// we may end up with more than we were asked for because
	// gitReq's parameter is minimum, not limit.
	if len(commits) > opts.Limit {
		commits = commits[:opts.Limit]
	}
	return
}

// possible values for "result" field of logCounter metric below.
const (
	cacheHit        = "hit"
	cacheMiss       = "miss"
	cacheFailure    = "failure-cache"
	decodingFailure = "failure-decoding"
)

var logCounter = metric.NewCounter(
	"luci/milo/git/log/cache",
	"The number of hits we get in git.Log",
	nil,
	field.String("result"), // for possible value see consts above.
	field.String("host"),
	field.String("project"),
	field.String("ref"))

var latencyMetric = metric.NewCumulativeDistribution(
	"luci/milo/git/log/latency",
	"Gitiles response latency",
	&types.MetricMetadata{Units: types.Milliseconds},
	distribution.DefaultBucketer,
	field.Bool("treeDiff"),
	field.String("host"),
	field.String("project"))

var gitHash = regexp.MustCompile("^[0-9a-fA-F]{40}$")

// logReq is the implementation of Log().
//
// Cached commits are keyed not only off of the requested commitish,
// but also ancestors, for example if received 100 ancestor
// commits C100, C99, .., C2, C1,
// cache with C99, C98, ... C1, too.
//
// The last byte of a cache value specifies how to interpret the rest:
//
//	0:       the rest is protobuf-marshalled gitilespb.LogResponse message
//	1-99:    the rest is byte id of a descendant commit that
//	         the commit-in-cache-key is reachable from.
//	         The last byte indicates the distance between the two commits.
//	100-255: reserved for future.
type logReq struct {
	factory   *implementation
	host      string
	project   string
	commitish string
	ancestor  string
	withFiles bool
	min       int // must be in [1..100]

	// fields below are set in call()

	commitishIsHash          bool
	commitishCacheKey        string
	commitishCacheExpiration time.Duration
}

func (l *logReq) call(c context.Context) ([]*gitpb.Commit, error) {
	if l.min < 1 || l.min > 100 {
		panic(fmt.Sprintf("invalid min %d", l.min))
	}

	c = logging.SetFields(c, logging.Fields{
		"host":      l.host,
		"project":   l.project,
		"commitish": l.commitish,
	})

	l.commitishIsHash = gitHash.MatchString(l.commitish)
	l.commitishCacheKey = l.mkCacheKey(c, l.commitish)
	l.commitishCacheExpiration = 12 * time.Hour
	if !l.commitishIsHash {
		// committish is not pinned, may move, so set a short expiration.
		l.commitishCacheExpiration = 30 * time.Second
	}

	cacheResult := ""
	defer func() {
		ref := l.commitish
		if l.commitishIsHash {
			ref = "PINNED"
		}
		logCounter.Add(c, 1, cacheResult, l.host, l.project, ref)
	}()

	cacheResult, commits, ok := l.readCache(c)
	if ok {
		return commits, nil
	}

	// cache miss, cache failure or corrupted cache.
	// Call Gitiles.
	req := &gitilespb.LogRequest{
		Project:    l.project,
		Committish: l.commitish,
		PageSize:   100,
		TreeDiff:   l.withFiles,
	}
	if l.ancestor != "" {
		req.ExcludeAncestorsOf = l.ancestor
	}
	logging.Infof(c, "gitiles(%q).Log(%#v)", l.host, req)
	client, err := l.factory.gitilesClient(c, l.host)
	if err != nil {
		return nil, err
	}
	start := clock.Now(c)
	res, err := client.Log(c, req)
	latency := float64(clock.Now(c).Sub(start).Nanoseconds()) / 1000000.0
	logging.Debugf(c, "gitiles took %fms", latency)
	latencyMetric.Add(c, latency, l.withFiles, l.host, l.project)
	if err != nil {
		return nil, errors.Annotate(err, "gitiles.Log").Err()
	}

	l.writeCache(c, res)
	return res.Log, nil
}

func (l *logReq) readCache(c context.Context) (cacheResult string, commits []*gitpb.Commit, ok bool) {
	conn, err := redisconn.Get(c)
	if err != nil {
		return cacheFailure, nil, false
	}
	defer conn.Close()

	e := l.commitishCacheKey
	dist := byte(0)
	maxDist := byte(100 - l.min)

	for {
		switch bytes, err := redis.Bytes(conn.Do("GET", e)); {
		case err == redis.ErrNil:
			logging.Infof(c, "cache miss")
			cacheResult = cacheMiss

		case err != nil:
			logging.WithError(err).Errorf(c, "cache failure")
			cacheResult = cacheFailure

		case len(bytes) == 0:
			logging.WithError(err).Errorf(c, "empty cache value at key %q", e)
			cacheResult = decodingFailure
		default:
			n := len(bytes)
			// see logReq for cache value format.
			data := bytes[:n-1]
			meta := bytes[n-1]
			switch {
			case meta == 0:
				var decoded gitilespb.LogResponse
				if err := proto.Unmarshal(data, &decoded); err != nil {
					logging.WithError(err).Errorf(c, "could not decode cached commits at key %q", e)
					cacheResult = decodingFailure
				} else {
					cacheResult = cacheHit
					commits = decoded.Log[dist:]
					ok = true
				}

			case meta >= 100:
				logging.WithError(err).Errorf(c, "unexpected last byte %d in cache value at key %q", meta, e)
				cacheResult = decodingFailure

			case dist+meta <= maxDist:
				// meta is distance, and the referenced cache is sufficient for reuse
				dist += meta
				descendant := hex.EncodeToString(data)
				logging.Debugf(c, "recursing into cache %s with distance %d", descendant, meta)
				e = l.mkCacheKey(c, descendant)
				// cacheResult is not set => continue the loop.
			default:
				// TODO(nodir): if we need commits C200..C100, but we have
				// C200->C250 here (C250 entry has C250..C150), instead of
				// discarding cache, reuse C200..C150 from C250 and fetch
				// C150..C50.
				logging.Debugf(c, "distance at key %q is too large", e)
				cacheResult = cacheMiss
			}
		}

		if cacheResult != "" {
			return
		}
	}
}

func (l *logReq) writeCache(c context.Context, res *gitilespb.LogResponse) {
	conn, err := redisconn.Get(c)
	if err != nil {
		logging.WithError(err).Errorf(c, "failed to get redis connection")
		return
	}
	defer conn.Close()

	updatedCacheCount := 0
	if len(res.Log) > 1 {
		// Also potentially cache with ancestors as cache keys.
		ancestorCacheKeys := make([]any, 0, len(res.Log)-1)
		for i := 1; i < len(res.Log) && len(res.Log[i-1].Parents) == 1; i++ {
			ancestorCacheKeys = append(ancestorCacheKeys, l.mkCacheKey(c, res.Log[i].Id))
		}
		ancestorCacheValues, err := redis.ByteSlices(conn.Do("MGET", ancestorCacheKeys...))
		if err != nil {
			logging.WithError(err).Errorf(c, "Failed to retrieve cache entry")
		}

		topCommitID, err := hex.DecodeString(res.Log[0].Id)
		if err != nil {
			logging.WithError(err).Errorf(c, "commit id %q is not a valid hex", res.Log[0].Id)
		} else {
			conn.Send("MULTI")

			// see logReq comment for cache value format.
			cacheValue, err := proto.Marshal(res)
			if err != nil {
				logging.WithError(err).Errorf(c, "failed to marshal gitiles log %s", res)
				return
			}
			cacheValue = append(cacheValue, 0)

			// cache with the commitish as cache key.
			conn.Send("SET", l.commitishCacheKey, cacheValue, "EX", l.commitishCacheExpiration.Seconds())
			updatedCacheCount += 1

			// cache with the commit hash as cache key too.
			cacheKey := l.mkCacheKey(c, res.Log[0].Id)
			if cacheKey != l.commitishCacheKey {
				conn.Send("SET", cacheKey, cacheValue, "EX", (12 * time.Hour).Seconds())
				updatedCacheCount += 1
			}

			for i, cacheValue := range ancestorCacheValues {
				dist := byte(i + 1)
				if len(cacheValue) > 0 && cacheValue[len(cacheValue)-1] <= dist {
					// This cache entry is not worse than what we can offer.
					continue
				}
				// We have data with a shorter distance.
				// see logReq comment for format of this cache value.
				cacheValue = make([]byte, len(topCommitID)+1)
				copy(cacheValue, topCommitID)
				cacheValue[len(cacheValue)-1] = dist
				conn.Send("SET", ancestorCacheKeys[i].(string), cacheValue, "EX", (12 * time.Hour).Seconds())
				updatedCacheCount += 1
			}
		}
	}

	// This could be potentially improved by using WATCH or Lua script,
	// but it would significantly complicate this code.
	_, err = conn.Do("EXEC")
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to cache gitiles log")
	} else {
		logging.Debugf(c, "wrote %d entries", updatedCacheCount)
	}
}

func (l *logReq) mkCacheKey(c context.Context, commitish string) string {
	// note: better not to include limit in the cache key.
	return fmt.Sprintf("git-log-%s|%s|%s|%s|%t", l.host, l.project, commitish,
		l.ancestor, l.withFiles)
}
