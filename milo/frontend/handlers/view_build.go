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

package handlers

import (
	"context"
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/buildbucket/bbperms"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gitpb "go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/xsrf"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/milo/frontend/handlers/ui"
	"go.chromium.org/luci/milo/internal/buildsource/buildbucket"
	"go.chromium.org/luci/milo/internal/utils"
	milopb "go.chromium.org/luci/milo/proto/v1"
	"go.chromium.org/luci/milo/rpc"
)

// handleLUCIBuild renders a LUCI build.
func handleLUCIBuild(c *router.Context) error {
	bid := &buildbucketpb.BuilderID{
		Project: c.Params.ByName("project"),
		Bucket:  c.Params.ByName("bucket"),
		Builder: c.Params.ByName("builder"),
	}
	numberOrID := c.Params.ByName("numberOrId")
	forceBlamelist := c.Request.FormValue("blamelist") != ""
	blamelistOpt := buildbucket.GetBlamelist
	if forceBlamelist {
		blamelistOpt = buildbucket.ForceBlamelist
	}

	// Redirect to short bucket names.
	if _, v2Bucket := bbv1.BucketNameToV2(bid.Bucket); v2Bucket != "" {
		// Parameter "bucket" is v1, e.g. "luci.chromium.try".
		u := *c.Request.URL
		u.Path = fmt.Sprintf("/p/%s/builders/%s/%s/%s", bid.Project, v2Bucket, bid.Builder, numberOrID)
		http.Redirect(c.Writer, c.Request, u.String(), http.StatusMovedPermanently)
	}

	br, err := prepareGetBuildRequest(bid, numberOrID)
	if err != nil {
		return err
	}

	bp, err := GetBuildPage(c, br, blamelistOpt)
	return renderBuild(c, bp, true, err)
}

// GetBuildPage fetches the full set of information for a Milo build page from Buildbucket.
// Including the blamelist and other auxiliary information.
func GetBuildPage(ctx *router.Context, br *buildbucketpb.GetBuildRequest, blamelistOpt buildbucket.BlamelistOption) (*ui.BuildPage, error) {
	now := timestamppb.New(clock.Now(ctx.Request.Context()))

	c := ctx.Request.Context()
	host, err := buildbucket.GetHost(c)
	if err != nil {
		return nil, err
	}
	client, err := buildbucket.BuildsClient(c, host, auth.AsUser)
	if err != nil {
		return nil, err
	}

	br.Fields = buildbucket.FullBuildMask
	b, err := client.GetBuild(c, br)
	if err != nil {
		return nil, utils.TagGRPC(c, err)
	}

	var blame []*ui.Commit
	var blameErr error
	switch blamelistOpt {
	case buildbucket.ForceBlamelist:
		blame, blameErr = getBlame(c, host, b, 55*time.Second)
	case buildbucket.GetBlamelist:
		blame, blameErr = getBlame(c, host, b, 1*time.Second)
	case buildbucket.NoBlamelist:
		break
	default:
		blameErr = errors.Reason("invalid blamelist option").Err()
	}

	realm := realms.Join(b.Builder.Project, b.Builder.Bucket)
	canCancel, err := auth.HasPermission(c, bbperms.BuildsCancel, realm, nil)
	if err != nil {
		return nil, err
	}
	canRetry, err := auth.HasPermission(c, bbperms.BuildsAdd, realm, nil)
	if err != nil {
		return nil, err
	}

	logging.Infof(c, "Got all the things")
	return &ui.BuildPage{
		Build: ui.Build{
			Build: b,
			Now:   now,
		},
		Blame:           blame,
		BuildbucketHost: host,
		BlamelistError:  blameErr,
		ForcedBlamelist: blamelistOpt == buildbucket.ForceBlamelist,
		CanCancel:       canCancel,
		CanRetry:        canRetry,
	}, nil
}

// simplisticBlamelist returns a slice of ui.Commit for a build, and/or an error.
//
// HACK(iannucci) - Getting the frontend to render a proper blamelist will
// require some significant refactoring. To do this properly, we'll need:
//   - The frontend to get BuildSummary from the backend.
//   - BuildSummary to have a .PreviousBuild() API.
//   - The frontend to obtain the annotation streams itself (so it could see
//     the SourceManifest objects inside of them). Currently getRespBuild defers
//     to swarming's implementation of buildsource.ID.Get(), which only returns
//     the resp object.
func simplisticBlamelist(c context.Context, build *buildbucketpb.Build) (result []*ui.Commit, err error) {
	gc := build.GetInput().GetGitilesCommit()
	if gc == nil {
		return
	}

	svc := &rpc.MiloInternalService{
		GetGitilesClient: func(c context.Context, host string, as auth.RPCAuthorityKind) (gitilespb.GitilesClient, error) {
			// Override to use `auth.AsSessionUser` because we know the function is
			// called in the context of a cookie authenticated request not an actual
			// RPC.
			// This is a hack but the old build page should eventually go away once
			// buildbucket can handle raw builds.
			t, err := auth.GetRPCTransport(c, auth.AsSessionUser)
			if err != nil {
				return nil, err
			}
			client, err := gitiles.NewRESTClient(&http.Client{Transport: t}, host, false)
			if err != nil {
				return nil, err
			}

			return client, nil
		},
	}
	req := &milopb.QueryBlamelistRequest{
		Builder:       build.Builder,
		GitilesCommit: gc,
		PageSize:      100,
	}
	res, err := svc.QueryBlamelist(c, req)

	switch {
	case err == nil:
		// continue
	case status.Code(err) == codes.PermissionDenied:
		err = grpcutil.UnauthenticatedTag.Apply(err)
		return
	default:
		return
	}

	result = make([]*ui.Commit, 0, len(res.Commits)+1)
	for _, commit := range res.Commits {
		result = append(result, uiCommit(commit, protoutil.GitilesRepoURL(gc)))
	}
	logging.Infof(c, "Fetched %d commit blamelist from Gitiles", len(result))

	if res.NextPageToken != "" {
		result = append(result, &ui.Commit{
			Description: "<blame list capped at 100 commits>",
			Revision:    &ui.Link{},
			AuthorName:  "<blame list capped at 100 commits>",
		})
	}

	return
}

func uiCommit(commit *gitpb.Commit, repoURL string) *ui.Commit {
	res := &ui.Commit{
		AuthorName:  commit.Author.Name,
		AuthorEmail: commit.Author.Email,
		Repo:        repoURL,
		Description: commit.Message,

		// TODO(iannucci): this use of links is very sloppy; the frontend should
		// know how to render a Commit without having Links embedded in it.
		Revision: ui.NewLink(
			commit.Id,
			repoURL+"/+/"+commit.Id, fmt.Sprintf("commit by %s", commit.Author.Email)),
	}
	res.CommitTime = commit.Committer.Time.AsTime()
	res.File = make([]string, 0, len(commit.TreeDiff))
	for _, td := range commit.TreeDiff {
		// If a file was moved, there is both an old and a new path, from which we
		// take only the new path.
		// If a file was deleted, its new path is /dev/null. In that case, we're
		// only interested in the old path.
		switch {
		case td.NewPath != "" && td.NewPath != "/dev/null":
			res.File = append(res.File, td.NewPath)
		case td.OldPath != "":
			res.File = append(res.File, td.OldPath)
		}
	}
	return res
}

// getBlame fetches blame information from Gitiles.
// This requires the BuildSummary to be indexed in Milo.
func getBlame(c context.Context, host string, b *buildbucketpb.Build, timeout time.Duration) ([]*ui.Commit, error) {
	nc, cancel := context.WithTimeout(c, timeout)
	defer cancel()
	commit := b.GetInput().GetGitilesCommit()
	// No commit? No blamelist.
	if commit == nil {
		return nil, nil
	}

	return simplisticBlamelist(nc, b)
}

// renderBuild is a shortcut for rendering build or returning err if it is not nil.
func renderBuild(c *router.Context, bp *ui.BuildPage, showOptInBanner bool, err error) error {
	if err != nil {
		return err
	}

	bp.StepDisplayPref = getStepDisplayPrefCookie(c)
	bp.ShowDebugLogsPref = getShowDebugLogsPrefCookie(c)

	var optInBannerHtml template.HTML
	if showOptInBanner {
		optInBannerHtml = bp.NewBuildPageOptInHTML()
	}

	templates.MustRender(c.Request.Context(), c.Writer, "pages/build.html", templates.Args{
		"BuildPage":      bp,
		"RetryRequestID": rand.Int31(),
		"XsrfTokenField": xsrf.TokenField(c.Request.Context()),
		"BannerHTML":     optInBannerHtml,
	})
	return nil
}

// redirectLUCIBuild redirects to a canonical build URL
// e.g. to /p/{project}/builders/{bucket}/{builder}/{number or id}.
func redirectLUCIBuild(c *router.Context) error {
	id, err := parseBuildID(c.Params.ByName("id"))
	if err != nil {
		return err
	}
	builder, number, err := buildbucket.GetBuilderID(c.Request.Context(), id)
	if err != nil {
		return err
	}
	numberOrID := fmt.Sprintf("%d", number)
	if number == 0 {
		numberOrID = fmt.Sprintf("b%d", id)
	}

	u := fmt.Sprintf("/p/%s/builders/%s/%s/%s?%s", builder.Project, builder.Bucket, builder.Builder, numberOrID, c.Request.URL.RawQuery)
	http.Redirect(c.Writer, c.Request, u, http.StatusMovedPermanently)
	return nil
}

func handleGetRelatedBuildsTable(c *router.Context) error {
	rbt, err := getRelatedBuilds(c)
	if err != nil {
		return err
	}
	templates.MustRender(c.Request.Context(), c.Writer, "widgets/related_builds_table.html", templates.Args{
		"RelatedBuildsTable": rbt,
	})
	return nil
}

func getRelatedBuilds(c *router.Context) (*ui.RelatedBuildsTable, error) {
	idInput := c.Params.ByName("id")

	id, err := strconv.ParseInt(idInput, 10, 64)
	if err != nil {
		return nil, errors.Annotate(err, "bad build id").Tag(grpcutil.InvalidArgumentTag).Err()
	}
	rbt, err := buildbucket.GetRelatedBuildsTable(c.Request.Context(), id)
	if err != nil {
		return nil, errors.Annotate(err, "error when getting related builds table").Err()
	}
	return rbt, nil
}

func getStepDisplayPrefCookie(c *router.Context) ui.StepDisplayPref {
	switch cookie, err := c.Request.Cookie("stepDisplayPref"); err {
	case nil:
		return ui.StepDisplayPref(cookie.Value)
	case http.ErrNoCookie:
		return ui.StepDisplayDefault
	default:
		logging.WithError(err).Errorf(c.Request.Context(), "failed to read stepDisplayPref cookie")
		return ui.StepDisplayDefault
	}
}

func getShowNewBuildPageCookie(c *router.Context) bool {
	switch cookie, err := c.Request.Cookie("showNewBuildPage"); err {
	case nil:
		return cookie.Value == "true"
	case http.ErrNoCookie:
		return true
	default:
		logging.WithError(err).Errorf(c.Request.Context(), "failed to read showNewBuildPage cookie")
		return true
	}
}

func getShowDebugLogsPrefCookie(c *router.Context) bool {
	switch cookie, err := c.Request.Cookie("showDebugLogsPref"); err {
	case nil:
		return cookie.Value == "true"
	case http.ErrNoCookie:
		return false
	default:
		logging.WithError(err).Errorf(c.Request.Context(), "failed to read showDebugLogsPref cookie")
		return false
	}
}

// parseBuildID parses build ID from string.
func parseBuildID(idStr string) (id int64, err error) {
	// Verify it is an int64.
	id, err = strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		err = errors.Annotate(err, "invalid id").Tag(grpcutil.InvalidArgumentTag).Err()
	}
	return
}

func prepareGetBuildRequest(builderID *buildbucketpb.BuilderID, numberOrID string) (*buildbucketpb.GetBuildRequest, error) {
	br := &buildbucketpb.GetBuildRequest{}
	if strings.HasPrefix(numberOrID, "b") {
		id, err := parseBuildID(numberOrID[1:])
		if err != nil {
			return nil, err
		}
		br.Id = id
	} else {
		number, err := strconv.ParseInt(numberOrID, 10, 32)
		if err != nil {
			return nil, errors.Annotate(err, "bad build number").Tag(grpcutil.InvalidArgumentTag).Err()
		}
		br.Builder = builderID
		br.BuildNumber = int32(number)
	}
	return br, nil
}
