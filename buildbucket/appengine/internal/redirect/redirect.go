// Copyright 2024 The LUCI Authors.
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

// Package redirect handles the URLs which need a redirection.
package redirect

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
)

// InstallHandlers adds routes handlers which need redirections.
func InstallHandlers(r *router.Router) {
	r.GET("/build/*BuildID", nil, handleViewBuild)
	r.GET("/builds/*BuildID", nil, handleViewBuild)
	r.GET("/log/*BuildID/*StepName", nil, handleViewBuild)
}

func handleViewLog(c *router.Context) {
	ctx := c.Request.Context()
	bID, err := strconv.Atoi(c.Params.ByName("BuildID"))
	if err != nil {
		replyError(c, err, "invalid build id", http.StatusBadRequest)
		return
	}

	bld := getBuild(c, bID)
	if bld == nil {
		return
	}
	stepName := c.Params.ByName("StepName")
	buildSteps := &model.BuildSteps{Build: datastore.KeyForObj(ctx, bld)}
	switch err := datastore.Get(ctx, buildSteps); {
	case errors.Contains(err, datastore.ErrNoSuchEntity):
		replyError(c, nil, "no steps found", http.StatusNotFound)
		return
	case err != nil:
		replyError(c, err, "error in fetching steps", http.StatusInternalServerError)
		return
	}

	steps, err := buildSteps.ToProto(ctx)
	if err != nil {
		replyError(c, err, "failed to parse steps", http.StatusInternalServerError)
		return
	}

	logName := c.Request.URL.Query().Get("log")
	if logName == "" {
		logName = "stdout"
	}
	logURL := findLogURL(stepName, logName, steps)
	if logURL == "" {
		replyError(c, nil, fmt.Sprintf("view url for log %q in step %q in build %d not found", logName, stepName, bID), http.StatusNotFound)
		return
	}
	http.Redirect(c.Writer, c.Request, logURL, http.StatusFound)
}

func findLogURL(stepName, logName string, steps []*pb.Step) string {
	for _, step := range steps {
		if step.GetName() == stepName {
			for _, log := range step.Logs {
				if log.GetName() == logName {
					return log.ViewUrl
				}
			}
			break
		}
	}
	return ""
}

// handleViewBuild redirects to Milo build page.
func handleViewBuild(c *router.Context) {
	ctx := c.Request.Context()
	bID, err := strconv.Atoi(c.Params.ByName("BuildID"))
	if err != nil {
		replyError(c, err, "invalid build id", http.StatusBadRequest)
		return
	}

	bld := getBuild(c, bID)
	if bld == nil {
		return
	}

	buildURL, err := getBuildURL(ctx, bld)
	if err != nil {
		replyError(c, err, "failed to generate the build url", http.StatusInternalServerError)
		return
	}
	http.Redirect(c.Writer, c.Request, buildURL, http.StatusFound)
}

// getBuild will return a build.
// For the unfounded build or a build that user has no access, it will directly reply http error.
// For anonymous user, it will redirect to the login page.
func getBuild(c *router.Context, bID int) *model.Build {
	ctx := c.Request.Context()
	bld, err := common.GetBuild(ctx, int64(bID))
	if err != nil {
		if s := status.Convert(err); s != nil {
			replyError(c, err, s.Message(), grpcutil.CodeStatus(s.Code()))
			return nil
		}
		replyError(c, err, "failed to get the build", http.StatusInternalServerError)
		return nil
	}

	if _, err := perm.GetFirstAvailablePerm(ctx, bld.Proto.Builder, bbperms.BuildsGet, bbperms.BuildsGetLimited); err != nil {
		// For anonymous users, redirect to the login page
		if caller := auth.CurrentIdentity(ctx); caller == identity.AnonymousIdentity {
			loginURL, err := auth.LoginURL(ctx, c.Request.URL.RequestURI())
			if err != nil {
				replyError(c, err, "failed to generate the login url", http.StatusInternalServerError)
				return nil
			}
			http.Redirect(c.Writer, c.Request, loginURL, http.StatusFound)
			return nil
		}

		if s := status.Convert(err); s != nil {
			replyError(c, err, s.Message(), grpcutil.CodeStatus(s.Code()))
			return nil
		}
		replyError(c, err, "failed to check perm", http.StatusInternalServerError)
		return nil
	}
	return bld
}

func getBuildURL(ctx context.Context, bld *model.Build) (string, error) {
	if bld.URL != "" {
		return bld.URL, nil
	}
	globalCfg, err := config.GetSettingsCfg(ctx)
	if err != nil {
		return "", err
	}

	if bld.BackendTarget != "" {
		for _, backendSetting := range globalCfg.Backends {
			if backendSetting.Target == bld.BackendTarget {
				if backendSetting.GetFullMode().GetRedirectToTaskPage() {
					bInfra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, bld)}
					if err := datastore.Get(ctx, bInfra); err != nil {
						return "", err
					}
					if bInfra.Proto.Backend.GetTask().GetLink() != "" {
						return bInfra.Proto.Backend.Task.Link, nil
					}
					logging.Errorf(ctx, "build %d had GetRedirectToTaskPage set to true but no task link was found", bld.ID)
				}
			}
		}
	}
	return fmt.Sprintf("https://%s/b/%d", globalCfg.Swarming.MiloHostname, bld.ID), nil
}

// replyError convert the provided error to http error and also logs the error.
func replyError(c *router.Context, err error, message string, code int) {
	if code < 500 {
		// User side error. Log it to info level.
		logging.Infof(c.Request.Context(), "%s: %s", message, err)
	} else {
		logging.Errorf(c.Request.Context(), "%s: %s", message, err)
	}

	http.Error(c.Writer, message, code)
}
