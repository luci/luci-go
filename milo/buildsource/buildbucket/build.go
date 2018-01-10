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
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/golang/protobuf/ptypes"
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/milo/buildsource/swarming"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/frontend/ui"
)

// BuildID implements buildsource.ID, and is the buildbucket notion of a build.
// It references a buildbucket build which may reference a swarming build.
type BuildID struct {
	// Project is the project which the build ID is supposed to reside in.
	Project string

	// Address is the Buildbucket's build address (required)
	Address string
}

var ErrNotFound = errors.Reason("Build not found").Tag(common.CodeNotFound).Err()

// GetSwarmingID returns the swarming task ID of a buildbucket build.
func GetSwarmingID(c context.Context, buildAddress string) (*swarming.BuildID, *model.BuildSummary, error) {
	host, err := getHost(c)
	if err != nil {
		return nil, nil, err
	}

	bs := &model.BuildSummary{BuildKey: MakeBuildKey(c, host, buildAddress)}
	switch err := datastore.Get(c, bs); err {
	case nil:
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
		// continue to the fallback code below.

	case datastore.ErrNoSuchEntity:
		// continue to the fallback code below.

	default:
		return nil, nil, err
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
	build, err := buildbucket.GetByAddress(c, client, buildAddress)
	switch {
	case err != nil:
		return nil, nil, errors.Annotate(err, "could not get build at %q", buildAddress).Err()
	case build == nil:
		return nil, nil, errors.Reason("build at %q not found", buildAddress).Tag(common.CodeNotFound).Err()
	}

	shost := build.Tags.Get("swarming_hostname")
	sid := build.Tags.Get("swarming_task_id")
	if shost == "" || sid == "" {
		return nil, nil, errors.New("not a valid LUCI build")
	}
	return &swarming.BuildID{Host: shost, TaskID: sid}, nil, nil
	// }}

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
func mixInSimplisticBlamelist(c context.Context, build *model.BuildSummary, rb *ui.MiloBuild) error {
	_, commits, err := build.PreviousByGitilesCommit(c)
	switch {
	case err == nil:
	case err == model.ErrUnknownPreviousBuild:
		return nil
	case gitiles.HTTPStatus(err) == http.StatusForbidden:
		return common.CodeUnauthorized.Tag().Apply(err)
	default:
		return err
	}

	gc := build.GitilesCommit()
	rb.Blame = make([]*ui.Commit, len(commits))
	for i, c := range commits {
		rb.Blame[i] = &ui.Commit{
			AuthorName:  c.Author.Name,
			AuthorEmail: c.Author.Email,
			Repo:        gc.RepoURL(),
			Description: c.Message,
			// TODO(iannucci): also include the diffstat.

			// TODO(iannucci): this use of links is very sloppy; the frontend should
			// know how to render a Commit without having Links embedded in it.
			Revision: ui.NewLink(
				c.Id,
				gc.RepoURL()+"/+/"+c.Id, fmt.Sprintf("commit by %s", c.Author.Email)),
		}

		rb.Blame[i].CommitTime, _ = ptypes.Timestamp(c.Committer.Time)
	}

	return nil
}

// getRespBuild fetches the full build state from Swarming and LogDog if
// available, otherwise returns an empty "pending build".
func getRespBuild(c context.Context, build *model.BuildSummary, sID *swarming.BuildID) (*ui.MiloBuild, error) {
	// TODO(nodir,hinoka): squash getRespBuild with toMiloBuild.

	// TODO(nodir,hinoka,iannucci): use annotations directly without fetching swarming task
	ret, err := sID.Get(c)
	if err != nil {
		return nil, err
	}

	if build != nil {
		switch err := mixInSimplisticBlamelist(c, build, ret); {
		case common.ErrorTag.In(err) == common.CodeUnauthorized:
			logging.WithError(err).Warningf(c, "dropping blamelist; access is unauthorized")
		case err != nil:
			return nil, err
		}
	}

	return ret, nil
}

// getBuildSummary fetches a build summary where the Context URI matches the
// given address.
func GetBuildSummary(c context.Context, id int64) (*model.BuildSummary, error) {
	// The host is set to prod because buildbot is hardcoded to talk to prod.
	uri := fmt.Sprintf("buildbucket://cr-buildbucket.appspot.com/build/%d", id)
	bs := make([]*model.BuildSummary, 0, 1)
	q := datastore.NewQuery("BuildSummary").Eq("ContextURI", uri).Limit(1)
	switch err := datastore.GetAll(c, q, &bs); {
	case err != nil:
		return nil, common.ReplaceNSEWith(err.(errors.MultiError), ErrNotFound)
	case len(bs) == 0:
		return nil, ErrNotFound
	default:
		return bs[0], nil
	}
}

// Get returns a resp.MiloBuild based off of the buildbucket ID given by
// finding the coorisponding swarming build.
func (b *BuildID) Get(c context.Context) (*ui.MiloBuild, error) {
	sID, bs, err := GetSwarmingID(c, b.Address)
	if err != nil {
		return nil, err
	}
	return getRespBuild(c, bs, sID)
}

func (b *BuildID) GetLog(c context.Context, logname string) (text string, closed bool, err error) {
	return "", false, errors.New("buildbucket builds do not implement GetLog")
}
