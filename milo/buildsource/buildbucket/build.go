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
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/golang/protobuf/ptypes"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/proto"
	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	miloProto "go.chromium.org/luci/common/proto/milo"
	logDogTypes "go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/milo/buildsource/rawpresentation"
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
// TODO(hinoka): BuildInfo and Skia requires this.
// Remove this when buildbucket v2 is out and Skia is on Kitchen.
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

// simplisticBlamelist returns the ui.MiloBuild.Blame field from the
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
func simplisticBlamelist(c context.Context, build *model.BuildSummary) ([]*ui.Commit, error) {
	builds, commits, err := build.PreviousByGitilesCommit(c)
	switch {
	case err == nil:
	case err == model.ErrUnknownPreviousBuild:
		return nil, nil
	case status.Code(err) == codes.PermissionDenied:
		return nil, common.CodeUnauthorized.Tag().Apply(err)
	default:
		return nil, err
	}

	gc := build.GitilesCommit()
	result := make([]*ui.Commit, 0, len(commits)+1)
	for _, c := range commits {
		commit := &ui.Commit{
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

		commit.CommitTime, _ = ptypes.Timestamp(c.Committer.Time)
		result = append(result, commit)
	}

	logging.Infof(c, "Fetched %d commit blamelist from Gitiles", len(result))

	// this means that there were more than 50 commits in-between.
	if len(builds) == 0 {
		result = append(result, &ui.Commit{
			Description: "<blame list capped at 50 commits>",
			Revision:    &ui.Link{},
		})
	}

	return result, nil
}

// extractEmail extracts the CL author email from a build message, if available.
// TODO(hinoka, nodir): Delete after buildbucket v2.
func extractEmail(msg *bbv1.ApiCommonBuildMessage) (string, error) {
	var params struct {
		Changes []struct {
			Author struct{ Email string }
		}
	}
	if msg.ParametersJson != "" {
		if err := json.NewDecoder(strings.NewReader(msg.ParametersJson)).Decode(&params); err != nil {
			return "", errors.Annotate(err, "failed to parse parameters_json").Err()
		}
	}
	if len(params.Changes) == 1 {
		return params.Changes[0].Author.Email, nil
	}
	return "", nil
}

// extractInfo extracts the resulting info text from a build message.
// TODO(hinoka, nodir): Delete after buildbucket v2.
func extractInfo(msg *bbv1.ApiCommonBuildMessage) (string, error) {
	var resultDetails struct {
		// TODO(nodir,iannucci): define a proto for build UI data
		UI struct {
			Info string
		}
	}
	if msg.ResultDetailsJson != "" {
		if err := json.NewDecoder(strings.NewReader(msg.ResultDetailsJson)).Decode(&resultDetails); err != nil {
			return "", errors.Annotate(err, "failed to parse result_details_json").Err()
		}
	}
	return resultDetails.UI.Info, nil
}

// toMiloBuild converts a buildbucket build to a milo build.
// In case of an error, returns a build with a description of the error
// and logs the error.
func toMiloBuild(c context.Context, msg *bbv1.ApiCommonBuildMessage) (*ui.MiloBuild, error) {
	// Parse the build message into a buildbucket.Build struct, filling in the
	// input and output properties that we expect to receive.
	var b buildbucket.Build
	var props struct {
		Revision string `json:"revision"`
	}
	var resultDetails struct {
		GotRevision string `json:"got_revision"`
	}
	b.Input.Properties = &props
	b.Output.Properties = &resultDetails
	if err := b.ParseMessage(msg); err != nil {
		return nil, err
	}
	// TODO(hinoka,nodir): Replace the following lines after buildbucket v2.
	info, err := extractInfo(msg)
	if err != nil {
		return nil, err
	}
	email, err := extractEmail(msg)
	if err != nil {
		return nil, err
	}

	result := &ui.MiloBuild{
		Summary: ui.BuildComponent{
			PendingTime:   ui.NewInterval(c, b.CreationTime, b.StartTime),
			ExecutionTime: ui.NewInterval(c, b.StartTime, b.CompletionTime),
			Status:        parseStatus(b.Status),
		},
		Trigger: &ui.Trigger{},
	}
	// Add in revision information, if available.
	if resultDetails.GotRevision != "" {
		result.Trigger.Commit.Revision = ui.NewEmptyLink(resultDetails.GotRevision)
	}
	if props.Revision != "" {
		result.Trigger.Commit.RequestRevision = ui.NewEmptyLink(props.Revision)
	}

	if b.Experimental {
		result.Summary.Text = []string{"Experimental"}
	}
	if info != "" {
		result.Summary.Text = append(result.Summary.Text, strings.Split(info, "\n")...)
	}

	for _, bs := range b.BuildSets {
		// ignore rietveld.
		cl, ok := bs.(*buildbucketpb.GerritChange)
		if !ok {
			continue
		}

		// support only one CL per build.
		result.Blame = []*ui.Commit{{
			Changelist:      ui.NewPatchLink(cl),
			RequestRevision: ui.NewLink(props.Revision, "", fmt.Sprintf("request revision %s", props.Revision)),
		}}

		result.Blame[0].AuthorEmail = email
		break
	}

	// Sprinkle in some extra information from Swarming
	swarmingTags := strpair.ParseMap(b.Tags["swarming_tag"])
	swarming.AddBanner(result, swarmingTags)
	swarming.AddRecipeLink(result, swarmingTags)
	swarming.AddProjectInfo(result, swarmingTags)
	task := b.Tags.Get("swarming_task_id")
	result.Summary.Source = ui.NewLink(
		"Task "+task,
		swarming.TaskPageURL(b.Tags.Get("swarming_hostname"), task).String(),
		"Swarming task page for task "+task)
	// TODO(hinoka): Add in Swarming Bot ID when it is surfaced in buildbucket.

	if b.Number != nil {
		numStr := strconv.Itoa(*b.Number)
		result.Summary.Label = ui.NewLink(
			numStr,
			fmt.Sprintf("/p/%s/builders/%s/%s/%s", b.Project, b.Bucket, b.Builder, numStr),
			fmt.Sprintf("build #%s", numStr))
		result.Summary.ParentLabel = ui.NewLink(
			b.Builder,
			fmt.Sprintf("/p/%s/builders/%s/%s", b.Project, b.Bucket, b.Builder),
			fmt.Sprintf("builder %s", b.Builder))
	} else {
		idStr := strconv.FormatInt(b.ID, 10)
		result.Summary.Label = ui.NewLink(
			idStr,
			fmt.Sprintf("/p/%s/builds/b%s", b.Project, idStr),
			fmt.Sprintf("build #%s", idStr))
	}
	return result, nil
}

// getUIBuild fetches the full build state from Swarming and LogDog if
// available, otherwise returns an empty "pending build".
// TODO(hinoka): Remove this when Skia migrates to Kitchen.
func getUIBuild(c context.Context, build *model.BuildSummary, sID *swarming.BuildID) (*ui.MiloBuild, error) {
	ret, err := sID.Get(c)
	if err != nil {
		return nil, err
	}

	if build != nil {
		switch blame, err := simplisticBlamelist(c, build); {
		case err != nil:
			ret.Blame = []*ui.Commit{{
				Description: fmt.Sprintf("Failed to fetch blame information\n%s", err.Error()),
			}}
		default:
			ret.Blame = blame
		}
		if buildName := build.GetBuildName(); buildName != "" {
			ret.Summary.Label = ui.NewEmptyLink(buildName)
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

func getBuildbucketBuild(c context.Context, address string) (*bbv1.ApiCommonBuildMessage, error) {
	host, err := getHost(c)
	if err != nil {
		return nil, err
	}

	client, err := newBuildbucketClient(c, host)
	if err != nil {
		return nil, err
	}
	// This runs a search RPC against BuildBucket, but it is optimized for speed.
	build, err := bbv1.GetByAddress(c, client, address)
	switch {
	case err != nil:
		return nil, errors.Annotate(err, "could not get build at %q", address).Err()
	case build == nil:
		return nil, errors.Reason("build at %q not found", address).Tag(common.CodeNotFound).Err()
	default:
		return build, nil
	}
}

// getStep fetches returns the Step annoations from LogDog.
func getStep(c context.Context, bbBuildMessage *bbv1.ApiCommonBuildMessage) (*logDogTypes.StreamAddr, *miloProto.Step, error) {
	swarmingTags := strpair.ParseMap(bbBuildMessage.Tags)["swarming_tag"]
	logLocation := strpair.ParseMap(swarmingTags).Get("log_location")
	if logLocation == "" {
		return nil, nil, errors.New("Build is missing log_location")
	}
	addr, err := logDogTypes.ParseURL(logLocation)
	if err != nil {
		return nil, nil, errors.Annotate(err, "%s is invalid", addr).Err()
	}

	step, err := rawpresentation.ReadAnnotations(c, addr)
	return addr, step, err
}

// getBlame fetches blame information from Gitiles.  This requires the
// BuildSummary to be indexed in Milo.
func getBlame(c context.Context, msg *bbv1.ApiCommonBuildMessage) ([]*ui.Commit, error) {
	host, err := getHost(c)
	if err != nil {
		return nil, err
	}
	tags := strpair.ParseMap(msg.Tags)
	bSet, _ := tags["buildset"]
	bs := &model.BuildSummary{
		BuildKey:  MakeBuildKey(c, host, tags.Get("build_address")),
		BuildSet:  bSet,
		BuilderID: fmt.Sprintf("buildbucket/%s/%s", msg.Bucket, tags.Get("builder")),
	}
	return simplisticBlamelist(c, bs)
}

// Get returns a resp.MiloBuild based off of the Buildbucket address.
// It performs the following actions:
// * Fetch the full Buildbucket build from Buildbucket
// * Fetch the step information from LogDog
// * Fetch the blame information from Gitiles
// This only errors out on failures to reach BuildBucket.  LogDog and Gitiles errors
// are surfaced in the UI.
// TODO(hinoka): Some of this can be done concurrently.  Investigate if this call
// takes >500ms on average.
func (b *BuildID) Get(c context.Context) (*ui.MiloBuild, error) {
	bbBuildMessage, err := getBuildbucketBuild(c, b.Address)
	if err != nil {
		return nil, err
	}

	mb, err := toMiloBuild(c, bbBuildMessage)
	if err != nil {
		return nil, err
	}

	// Add step information from LogDog.  If this fails, we still have perfectly
	// valid information from Buildbucket, so just annotate the build with the
	// error and continue.
	if addr, step, err := getStep(c, bbBuildMessage); err == nil {
		ub := rawpresentation.NewURLBuilder(addr)
		mb.Components, mb.PropertyGroup = rawpresentation.SubStepsToUI(c, ub, step.Substep)
	} else {
		// TODO(hinoka): This might be better placed in a error butterbar.
		mb.Components = append(mb.Components, &ui.BuildComponent{
			Label:  ui.NewEmptyLink("Failed to fetch step information from LogDog"),
			Text:   strings.Split(err.Error(), "\n"),
			Status: model.InfraFailure,
		})
	}

	// Add blame information.  If this fails, just add in a placeholder with an error.
	if blame, err := getBlame(c, bbBuildMessage); err == nil {
		mb.Blame = blame
	} else {
		mb.Blame = []*ui.Commit{{
			Description: fmt.Sprintf("Failed to fetch blame information\n%s", err.Error()),
		}}
	}
	return mb, nil
}

func (b *BuildID) GetLog(c context.Context, logname string) (text string, closed bool, err error) {
	return "", false, errors.New("buildbucket builds do not implement GetLog")
}
