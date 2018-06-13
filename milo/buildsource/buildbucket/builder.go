// Copyright 2016 The LUCI Authors.
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

package buildbucket

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"

	"go.chromium.org/gae/service/memcache"
	bb "go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/proto"
	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/frontend/ui"
)

// BuilderID represents a buildbucket builder.  We wrap the underlying representation
// since we represent builder IDs slightly differently in Milo vs. Buildbucket.
// I.E. Builders can source from either BuildBot or Buildbucket.
type BuilderID struct {
	// BuilderID is the buildbucket v2 representation of the builder ID.  Note
	// that the v2 representation uses short bucket names.
	buildbucketpb.BuilderID
}

// NewBuilderID does what it says.
func NewBuilderID(v1Bucket, builder string) (bid BuilderID) {
	bid.Project, bid.Bucket = bb.BucketNameToV2(v1Bucket)
	bid.Builder = builder
	return
}

// V1Bucket returns the buildbucket v1 representation of the bucket name, which
// is what we use in Milo.
func (b BuilderID) V1Bucket() string {
	return fmt.Sprintf("luci.%s.%s", b.Project, b.Bucket)
}

// String returns the canonical format of BuilderID.
func (b BuilderID) String() string {
	return fmt.Sprintf("buildbucket/%s/%s", b.V1Bucket(), b.Builder)
}

// fetchBuilds fetches builds given a criteria.
// The returned builds are sorted by build creation descending.
// count defines maximum number of builds to fetch; if <0, defaults to 100.
func fetchBuilds(c context.Context, client *bbv1.Service, bid BuilderID,
	status string, limit int, cursor string) ([]*bbv1.ApiCommonBuildMessage, string, error) {

	c, _ = context.WithTimeout(c, bbRPCTimeout)
	search := client.Search()
	search.Context(c)
	search.Bucket(bid.V1Bucket())
	search.Status(status)
	search.Tag(strpair.Format(bbv1.TagBuilder, bid.Builder))
	search.IncludeExperimental(true)
	search.StartCursor(cursor)

	if limit < 0 {
		limit = 100
	}

	start := clock.Now(c)
	msgs, cursor, err := search.Fetch(limit, nil)
	if err != nil {
		return nil, "", err
	}
	logging.Infof(c, "Fetched %d %s builds in %s", len(msgs), status, clock.Since(c, start))
	return msgs, cursor, nil
}

// ensureDefined returns common.CodeNotFound tagged error if a builder is not
// defined in its swarmbucket.
func ensureDefined(c context.Context, host string, bid BuilderID) error {
	client, err := newSwarmbucketClient(c, host)
	if err != nil {
		return err
	}
	getBuilders := client.GetBuilders()
	getBuilders.Bucket(bid.V1Bucket())
	getBuilders.Fields(googleapi.Field("buckets/(builders/name,name)"))
	res, err := getBuilders.Do()
	if err != nil {
		return err
	}

	for _, bucket := range res.Buckets {
		if bucket.Name != bid.V1Bucket() {
			continue // defensive programming; shouldn't happen in practice.
		}
		for _, builder := range bucket.Builders {
			if builder.Name == bid.Builder {
				return nil
			}
		}
	}
	return errors.Reason("builder %q not found", bid.Builder).Tag(common.CodeNotFound).Err()
}

func getDebugBuilds(c context.Context, bid BuilderID, maxCompletedBuilds int, target *ui.Builder) error {
	// ../buildbucket below assumes that
	// - this code is not executed by tests outside of this dir
	// - this dir is a sibling of frontend dir
	resFile, err := os.Open(filepath.Join(
		"..", "buildbucket", "testdata", bid.V1Bucket(), bid.Builder+".json"))
	if err != nil {
		return err
	}
	defer resFile.Close()

	res := &bbv1.ApiSearchResponseMessage{}
	if err := json.NewDecoder(resFile).Decode(res); err != nil {
		return err
	}

	for _, bb := range res.Builds {
		mb, err := ToMiloBuild(c, bb, false)
		if err != nil {
			return err
		}
		bs := mb.BuildSummary()
		switch mb.Summary.Status {
		case model.NotRun:
			target.PendingBuilds = append(target.PendingBuilds, bs)

		case model.Running:
			target.CurrentBuilds = append(target.CurrentBuilds, bs)

		case model.Success, model.Failure, model.InfraFailure, model.Warning:
			if len(target.FinishedBuilds) < maxCompletedBuilds {
				target.FinishedBuilds = append(target.FinishedBuilds, bs)
			}

		default:
			panic("impossible")
		}
	}
	return nil
}

func getHost(c context.Context) (string, error) {
	settings := common.GetSettings(c)
	if settings.Buildbucket == nil || settings.Buildbucket.Host == "" {
		return "", errors.New("missing buildbucket host in settings")
	}
	return settings.Buildbucket.Host, nil
}

// backCursor implements bidirectional cursors with forward-only datastore
// cursors by storing a map for cursor -> prevCursor in memcache.
// backCursor returns a previous cursor given thisCursor, and caches thisCursor
// to be the previous cursor of nextCursor.
func backCursor(c context.Context, bid BuilderID, limit int, thisCursor, nextCursor string) string {
	memcacheKey := func(cursor string) string {
		key := fmt.Sprintf("%s:%d:%s", bid.String(), limit, cursor)
		blob := sha1.Sum([]byte(key))
		encoded := base64.StdEncoding.EncodeToString(blob[:])
		return "cursors:buildbucket_builders:" + encoded
	}

	prevCursor := ""
	if thisCursor != "" {
		if item, err := memcache.GetKey(c, memcacheKey(thisCursor)); err == nil {
			prevCursor = string(item.Value())
		}
	}
	if nextCursor != "" {
		item := memcache.NewItem(c, memcacheKey(nextCursor))
		if thisCursor == "" {
			item.SetValue([]byte("EMPTY"))
		} else {
			item.SetValue([]byte(thisCursor))
		}
		item.SetExpiration(24 * time.Hour)
		memcache.Set(c, item)
	}
	return prevCursor
}

// toMiloBuildsSummaries computes summary for each build in parallel.
func toMiloBuildsSummaries(c context.Context, msgs []*bbv1.ApiCommonBuildMessage) ([]*ui.BuildSummary, error) {
	result := make([]*ui.BuildSummary, len(msgs))
	// For each build, toMiloBuild may query Gerrit to fetch associated CL's
	// author email. Unfortunately, as of June 2018 Gerrit is often taking >5s to
	// report back. From UX PoV, author's email isn't the most important of
	// builder's page, so limit waiting time.
	c, _ = context.WithTimeout(c, 5*time.Second)
	return result, parallel.WorkPool(50, func(work chan<- func() error) {
		for i, m := range msgs {
			i := i
			m := m
			work <- func() error {
				mb, err := ToMiloBuild(c, m, false)
				if err != nil {
					return errors.Annotate(err, "failed to convert build %d to milo build", m.Id).Err()
				}
				result[i] = mb.BuildSummary()
				return nil
			}
		}
	})
}

// GetBuilder is used by buildsource.BuilderID.Get to obtain the resp.Builder.
func GetBuilder(c context.Context, bid BuilderID, limit int, cursor string) (*ui.Builder, error) {
	host, err := getHost(c)
	if err != nil {
		return nil, err
	}

	if limit < 0 {
		limit = 20
	}

	result := &ui.Builder{
		Name: bid.Builder,
	}
	if host == "debug" {
		return result, getDebugBuilds(c, bid, limit, result)
	}
	client, err := newBuildbucketClient(c, host)
	if err != nil {
		return nil, err
	}

	fetch := func(statusFilter string, limit int, cursor string) (result []*ui.BuildSummary, nextCursor string, err error) {
		msgs, nextCursor, err := fetchBuilds(c, client, bid, statusFilter, limit, cursor)
		if err != nil {
			logging.WithError(err).Errorf(c, "Could not fetch %s builds", statusFilter)
			return
		}
		result, err = toMiloBuildsSummaries(c, msgs)
		return
	}
	return result, parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() error {
			return ensureDefined(c, host, bid)
		}
		work <- func() (err error) {
			result.MachinePool, err = getPool(c, bid)
			return
		}
		work <- func() (err error) {
			result.PendingBuilds, _, err = fetch(bbv1.StatusScheduled, -1, "")
			return
		}
		work <- func() (err error) {
			result.CurrentBuilds, _, err = fetch(bbv1.StatusStarted, -1, "")
			return
		}
		work <- func() (err error) {
			result.FinishedBuilds, result.NextCursor, err = fetch(bbv1.StatusCompleted, limit, cursor)
			result.PrevCursor = backCursor(c, bid, limit, cursor, result.NextCursor) // Safe to do even with error.
			return
		}
	})
}
