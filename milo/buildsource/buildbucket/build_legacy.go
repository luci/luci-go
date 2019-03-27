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

package buildbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/buildbucket"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	miloProto "go.chromium.org/luci/common/proto/milo"
	logDogTypes "go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/milo/buildsource/rawpresentation"
	"go.chromium.org/luci/milo/buildsource/swarming"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/frontend/ui"
	"go.chromium.org/luci/milo/git"
)

// GetSwarmingTaskID returns the swarming task ID of a buildbucket build.
// TODO(hinoka): BuildInfo and Skia requires this.
// Remove this when buildbucket v2 is out and Skia is on Kitchen.
// TODO(nodir): delete this. It is used only in deprecated BuildInfo API.
func GetSwarmingTaskID(c context.Context, buildAddress string) (host, taskId string, err error) {
	host, err = getHost(c)
	if err != nil {
		return
	}

	bs := &model.BuildSummary{BuildKey: MakeBuildKey(c, host, buildAddress)}
	switch err = datastore.Get(c, bs); err {
	case nil:
		for _, ctx := range bs.ContextURI {
			u, err := url.Parse(ctx)
			if err != nil {
				continue
			}
			if u.Scheme == "swarming" && len(u.Path) > 1 {
				toks := strings.Split(u.Path[1:], "/")
				if toks[0] == "task" {
					return u.Host, toks[1], nil
				}
			}
		}
		// continue to the fallback code below.

	case datastore.ErrNoSuchEntity:
		// continue to the fallback code below.

	default:
		return
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
		return
	}
	build, err := buildbucket.GetByAddress(c, client, buildAddress)
	switch {
	case err != nil:
		err = errors.Annotate(err, "could not get build at %q", buildAddress).Err()
		return
	case build == nil:
		err = errors.Reason("build at %q not found", buildAddress).Tag(common.CodeNotFound).Err()
		return
	}

	host = build.Tags.Get("swarming_hostname")
	taskId = build.Tags.Get("swarming_task_id")
	if host == "" || taskId == "" {
		err = errors.New("not a valid LUCI build")
	}
	return
	// }}

}

// extractDetails extracts the following from a build message's ResultDetailsJson:
// * Build Info Text
// * Swarming Bot ID
// TODO(hinoka, nodir): Delete after buildbucket v2.
func extractDetails(msg *bbv1.ApiCommonBuildMessage) (info string, botID string, err error) {
	var resultDetails struct {
		// TODO(nodir,iannucci): define a proto for build UI data
		UI struct {
			Info string `json:"info"`
		} `json:"ui"`
		Swarming struct {
			TaskResult struct {
				BotID string `json:"bot_id"`
			} `json:"task_result"`
			BotDimensions struct {
				ID []string `json:"id"`
			} `json:"bot_dimensions"`
		} `json:"swarming"`
	}
	if msg.ResultDetailsJson != "" {
		err = json.NewDecoder(strings.NewReader(msg.ResultDetailsJson)).Decode(&resultDetails)
		info = resultDetails.UI.Info
		botID = resultDetails.Swarming.TaskResult.BotID
		if botID == "" && len(resultDetails.Swarming.BotDimensions.ID) > 0 {
			botID = resultDetails.Swarming.BotDimensions.ID[0]
		}
	}
	return
}

// toMiloBuildInMemory converts a buildbucket build to a milo build in memory.
// Does not make RPCs.
// In case of an error, returns a build with a description of the error
// and logs the error.
func toMiloBuildInMemory(c context.Context, msg *bbv1.ApiCommonBuildMessage) (*ui.MiloBuildLegacy, error) {
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
	info, botID, err := extractDetails(msg)
	if err != nil {
		return nil, err
	}

	// Now that all the data is parsed, put them all in the correct places.
	pendingEnd := b.StartTime
	if pendingEnd.IsZero() {
		// Maybe the build expired and never started.  Use the expiration time, if any.
		pendingEnd = b.CompletionTime
	}
	result := &ui.MiloBuildLegacy{
		Summary: ui.BuildComponent{
			PendingTime:   ui.NewInterval(c, b.CreationTime, pendingEnd),
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
	if botID != "" {
		result.Summary.Bot = ui.NewLink(
			botID,
			fmt.Sprintf(
				"https://%s/bot?id=%s", b.Tags.Get("swarming_hostname"), botID),
			fmt.Sprintf("Swarming Bot %s", botID))
	}

	for _, bs := range b.Tags[bbv1.TagBuildSet] {
		cl, ok := protoutil.ParseBuildSet(bs).(*buildbucketpb.GerritChange)
		if !ok {
			continue
		}

		// support only one CL per build.
		link := ui.NewPatchLink(cl)
		result.Blame = []*ui.Commit{{
			Changelist:      link,
			RequestRevision: ui.NewLink(props.Revision, "", fmt.Sprintf("request revision %s", props.Revision)),
		}}
		result.Trigger.Changelist = link

		result.Blame[0].AuthorEmail, err = git.Get(c).CLEmail(c, cl.Host, cl.Change)
		switch {
		case err == context.DeadlineExceeded:
			result.Blame[0].AuthorEmail = "<Gerrit took too long respond>"
			fallthrough
		case err != nil:
			logging.WithError(err).Errorf(c, "failed to load CL author for build %d", b.ID)
		}
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

	// Use v2 bucket names.
	_, bucket := buildbucket.BucketNameToV2(b.Bucket)

	result.Summary.ParentLabel = ui.NewLink(
		b.Builder,
		fmt.Sprintf("/p/%s/builders/%s/%s", b.Project, bucket, b.Builder),
		fmt.Sprintf("builder %s", b.Builder))
	if b.Number != nil {
		numStr := strconv.Itoa(*b.Number)
		result.Summary.Label = ui.NewLink(
			numStr,
			fmt.Sprintf("/p/%s/builders/%s/%s/%s", b.Project, bucket, b.Builder, numStr),
			fmt.Sprintf("build #%s", numStr))
	} else {
		idStr := strconv.FormatInt(b.ID, 10)
		result.Summary.Label = ui.NewLink(
			idStr,
			fmt.Sprintf("/b/%s", idStr),
			fmt.Sprintf("build #%s", idStr))
	}
	return result, nil
}

// GetRawBuild fetches a buildbucket build given its address.
func GetRawBuild(c context.Context, address string) (*bbv1.ApiCommonBuildMessage, error) {
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
	case build == nil && auth.CurrentUser(c).Identity == identity.AnonymousIdentity:
		return nil, errors.Reason("not logged in").Tag(common.CodeUnauthorized).Err()
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

// GetBuild is a shortcut for GetRawBuild and ToMiloBuild.
func GetBuildLegacy(c context.Context, address string, fetchFull bool) (*ui.MiloBuildLegacy, error) {
	bbBuildMessage, err := GetRawBuild(c, address)
	if err != nil {
		return nil, err
	}
	return ToMiloBuild(c, bbBuildMessage, fetchFull)
}

// ToMiloBuild converts a raw buildbucket build to a milo build.
//
// Returns an error only on failure to reach buildbucket.
// Other errors are surfaced in the returned build.
//
// TODO(hinoka): Some of this can be done concurrently. Investigate if this call
// takes >500ms on average.
// TODO(crbug.com/850113): stop loading steps from logdog.
func ToMiloBuild(c context.Context, b *bbv1.ApiCommonBuildMessage, fetchFull bool) (*ui.MiloBuildLegacy, error) {
	mb, err := toMiloBuildInMemory(c, b)
	if err != nil {
		return nil, err
	}

	if !fetchFull {
		return mb, nil
	}

	// Add step information from LogDog.  If this fails, we still have perfectly
	// valid information from Buildbucket, so just annotate the build with the
	// error and continue.
	if b.StartedTs != 0 {
		if addr, step, err := getStep(c, b); err == nil {
			ub := rawpresentation.NewURLBuilder(addr)
			mb.Components, mb.PropertyGroup = rawpresentation.SubStepsToUI(c, ub, step.Substep)
		} else if b.Status == bbv1.StatusCompleted {
			// TODO(hinoka): This might be better placed in a error butterbar.
			mb.Components = append(mb.Components, &ui.BuildComponent{
				Label:  ui.NewEmptyLink("Failed to fetch step information from LogDog"),
				Text:   strings.Split(err.Error(), "\n"),
				Status: model.InfraFailure,
			})
		}
	}

	// Add blame information.  If this fails, this add in a placeholder with an error.
	mb.Blame = getBlameLegacy(c, b)

	return mb, nil
}

// getBlameLegacy fetches blame information from Gitiles.  This requires the
// BuildSummary to be indexed in Milo.
func getBlameLegacy(c context.Context, msg *bbv1.ApiCommonBuildMessage) []*ui.Commit {
	host, err := getHost(c)
	if err != nil {
		return []*ui.Commit{{
			Description: fmt.Sprintf("Failed to fetch blame information\n%s", err.Error()),
		}}
	}
	tags := strpair.ParseMap(msg.Tags)
	bSet, _ := tags["buildset"]
	bid := NewBuilderID(msg.Bucket, tags.Get("builder"))
	bs := &model.BuildSummary{
		BuildKey:  MakeBuildKey(c, host, tags.Get("build_address")),
		BuildSet:  bSet,
		BuilderID: bid.String(),
	}
	blame, err := simplisticBlamelist(c, bs)
	if err != nil {
		logging.WithError(err).Warningf(c, "getting blamelist")
	}
	return blame
}
