// Copyright 2019 The LUCI Authors.
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
	"fmt"
	"net/http"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/milo/frontend/handlers/ui"
	"go.chromium.org/luci/milo/internal/buildsource/buildbucket"
	"go.chromium.org/luci/milo/internal/utils"
	"go.chromium.org/luci/server/router"
)

// cancelBuildHandler parses inputs from HTTP form, cancels the build with the given buildbucket build ID
// then redirects the users back to the original page.
func cancelBuildHandler(ctx *router.Context) error {
	id, reason, err := parseCancelBuildInput(ctx)
	if err != nil {
		return errors.Annotate(err, "error while parsing cancel build request input fields").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	if _, err := buildbucket.CancelBuild(ctx.Request.Context(), id, reason); err != nil {
		return err
	}

	http.Redirect(ctx.Writer, ctx.Request, ctx.Request.Referer(), http.StatusSeeOther)
	return nil
}

func parseCancelBuildInput(ctx *router.Context) (buildbucketID int64, reason string, err error) {
	if err := ctx.Request.ParseForm(); err != nil {
		return 0, "", errors.Annotate(err, "unable to parse cancel build form").Err()
	}

	buildbucketID, err = utils.ParseIntFromForm(ctx.Request.Form, "buildbucket-id", 10, 64)
	if err != nil {
		return 0, "", errors.Annotate(err, "invalid buildbucket-id").Err()
	}

	reason, err = utils.ReadExactOneFromForm(ctx.Request.Form, "reason")
	if err != nil {
		return 0, "", err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return 0, "", fmt.Errorf("reason cannot be empty")
	}

	return buildbucketID, reason, nil
}

// retryBuildHandler parses inputs from HTTP form, retries the build with the given buildbucket build ID
// then redirects the users to the retry build page.
func retryBuildHandler(ctx *router.Context) error {
	buildbucketID, requestID, err := parseRetryBuildInput(ctx)
	if err != nil {
		return errors.Annotate(err, "error while parsing retry build request input fields").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	build, err := buildbucket.RetryBuild(ctx.Request.Context(), buildbucketID, requestID)
	if err != nil {
		return err
	}
	uiBuild := ui.Build{
		Build: build,
	}
	http.Redirect(ctx.Writer, ctx.Request, uiBuild.Link().URL, http.StatusSeeOther)
	return nil
}

func parseRetryBuildInput(ctx *router.Context) (buildbucketID int64, retryRequestID string, err error) {
	if err := ctx.Request.ParseForm(); err != nil {
		return 0, "", errors.Annotate(err, "unable to parse retry build form").Err()
	}

	buildbucketID, err = utils.ParseIntFromForm(ctx.Request.Form, "buildbucket-id", 10, 64)
	if err != nil {
		return 0, "", errors.Annotate(err, "invalid buildbucket-id").Err()
	}

	retryRequestID, err = utils.ReadExactOneFromForm(ctx.Request.Form, "retry-request-id")
	if err != nil {
		return 0, "", err
	}
	retryRequestID = strings.TrimSpace(retryRequestID)
	if retryRequestID == "" {
		return 0, "", fmt.Errorf("retry request ID cannot be empty")
	}

	return buildbucketID, retryRequestID, nil
}
