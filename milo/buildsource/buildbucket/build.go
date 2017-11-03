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
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/golang/protobuf/ptypes"
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/milo/api/resp"
	"go.chromium.org/luci/milo/buildsource/swarming"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
)

// BuildID implements buildsource.ID, and is the buildbucket notion of a build.
// It references a buildbucket build which may reference a swarming build.
type BuildID struct {
	// Project is the project which the build ID is supposed to reside in.
	Project string

	// ID is the Buildbucket's build ID (required)
	ID string
}

// GetSwarmingID returns the swarming task ID of a buildbucket build.
func GetSwarmingID(c context.Context, id int64) (*swarming.BuildID, *model.BuildSummary, error) {
	host, err := getHost(c)
	if err != nil {
		return nil, nil, err
	}

	bs := &model.BuildSummary{BuildKey: MakeBuildKey(c, host, id)}
	switch err := datastore.Get(c, bs); err {
	case nil:
	case datastore.ErrNoSuchEntity:
		logging.Warningf(c, "failed to load BuildSummary for: %q", bs.BuildKey)
		return nil, nil, errors.New("could not find build", common.CodeNotFound)
	default:
		return nil, nil, err
	}

	for _, ctx := range bs.ContextURI {
		u, err := url.Parse(ctx)
		if err != nil {
			continue
		}
		if u.Scheme == "swarming" && len(u.Path) > 1 {
			toks := strings.Split(u.Path[1:], "/")
			if toks[0] == "task" {
				return &swarming.BuildID{Host: u.Host, TaskID: toks[1]}, bs, nil
			}
		}
	}

	// DEPRECATED(2017-12-01) {{
	// This makes an RPC to buildbucket to obtain the swarming task ID.
	// Now that we include this data in the BuildSummary.ContextUI we should never
	// need to do this extra RPC. However, we have this codepath in place for old
	// builds.
	//
	// After the deprecation date, this code can be removed; the only effect will
	// be that buildbucket builds before 2017-11-03 will not render.
	client, err := newBuildbucketClient(c, host)
	if err != nil {
		return nil, nil, err
	}
	resp, err := client.Get(id).Context(c).Do()
	if err != nil {
		return nil, nil, err
	}
	tags := strpair.ParseMap(resp.Build.Tags)
	if shost, sid := tags.Get("swarming_hostname"), tags.Get("swarming_task_id"); shost != "" && sid != "" {
		return &swarming.BuildID{Host: shost, TaskID: sid}, bs, nil
	}
	// }}

	return nil, nil, errors.New("no swarming task context")
}

// mixInSimplisticBlamelist populates the resp.Blame field from the
// commit/gitiles buildset (if any).
//
// HACK(iannucci) - Getting the frontend to render a proper blamelist will
// require some significant refactoring. To do this properly, we'll need:
//   * The frontend to get BuildSummary from the backend.
//   * BuildSummary to have a .PreviousBuild() API.
//   * The frontend to obtain the annotation streams itself (so it could see
//     the SourceManifest objects inside of them). Currently getRespBuild defers
//     to swarming's implementation of buildsource.ID.Get(), which only returns
//     the resp object.
func mixInSimplisticBlamelist(c context.Context, build *model.BuildSummary, rb *resp.MiloBuild) error {
	_, hist, err := build.PreviousByGitilesCommit(c)
	switch err {
	case nil:
	case model.ErrUnknownPreviousBuild:
		return nil
	default:
		return err
	}

	gc := build.GitilesCommit()
	rb.Blame = make([]*resp.Commit, len(hist.Commits))
	for i, c := range hist.Commits {
		rev := hex.EncodeToString(c.Hash)
		rb.Blame[i] = &resp.Commit{
			AuthorName:  c.AuthorName,
			AuthorEmail: c.AuthorEmail,
			Repo:        gc.RepoURL(),
			Description: c.Msg,
			// TODO(iannucci): also include the diffstat.

			// TODO(iannucci): this use of links is very sloppy; the frontend should
			// know how to render a Commit without having Links embedded in it.
			Revision: resp.NewLink(
				rev,
				gc.RepoURL()+"/+/"+rev, fmt.Sprintf("commit by %s", c.AuthorEmail)),
		}

		rb.Blame[i].CommitTime, _ = ptypes.Timestamp(c.CommitTime)
	}

	return nil
}

// getRespBuild fetches the full build state from Swarming and LogDog if
// available, otherwise returns an empty "pending build".
func getRespBuild(c context.Context, build *model.BuildSummary, sID *swarming.BuildID) (*resp.MiloBuild, error) {
	// TODO(nodir,hinoka): squash getRespBuild with toMiloBuild.

	if build.Summary.Status == model.NotRun {
		// Hasn't started yet, so definitely no build ready yet, return a pending
		// build.
		return &resp.MiloBuild{
			Summary: resp.BuildComponent{Status: model.NotRun},
		}, nil
	}

	// TODO(nodir,hinoka,iannucci): use annotations directly without fetching swarming task
	ret, err := sID.Get(c)
	if err != nil {
		return nil, err
	}

	if err := mixInSimplisticBlamelist(c, build, ret); err != nil {
		return nil, err
	}
	return ret, err
}

// Get returns a resp.MiloBuild based off of the buildbucket ID given by
// finding the coorisponding swarming build.
func (b *BuildID) Get(c context.Context) (*resp.MiloBuild, error) {
	id, err := strconv.ParseInt(b.ID, 10, 64)
	if err != nil {
		return nil, errors.Annotate(
			err, "%s is not a valid number", b.ID).Tag(common.CodeParameterError).Err()
	}

	sID, bs, err := GetSwarmingID(c, id)
	if err != nil {
		return nil, err
	}

	result, err := getRespBuild(c, bs, sID)
	if err != nil {
		return nil, err
	}

	if result.Trigger.Project != b.Project {
		return nil, errors.New("invalid project", common.CodeParameterError)
	}
	return result, nil
}

func (b *BuildID) GetLog(c context.Context, logname string) (text string, closed bool, err error) {
	return "", false, errors.New("buildbucket builds do not implement GetLog")
}
