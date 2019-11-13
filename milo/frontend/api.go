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

package frontend

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/frontend/ui"
	"go.chromium.org/luci/server/router"
)

// cancelBuildHandler parses inputs from HTTP form, cancels the build with the given buildbucket build ID
// then redirects the users back to the original page.
func cancelBuildHandler(ctx *router.Context) error {
	id, reason, err := parseCancelBuildInput(ctx)
	if err != nil {
		return errors.Annotate(err, "error while parsing cancel build request input fields").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	if _, err := buildbucket.CancelBuild(ctx.Context, id, reason); err != nil {
		return err
	}

	http.Redirect(ctx.Writer, ctx.Request, ctx.Request.Referer(), http.StatusSeeOther)
	return nil
}

func parseCancelBuildInput(ctx *router.Context) (int64, string, error) {
	if err := ctx.Request.ParseForm(); err != nil {
		return 0, "", errors.Annotate(err, "unable to parse cancel build form").Err()
	}

	idInput := ctx.Request.Form["buildbucket-id"]
	if len(idInput) != 1 || idInput[0] == "" {
		return 0, "", fmt.Errorf("invalid or missing buildbucket-id; expected an integer. actual value: %v", idInput)
	}
	id, err := strconv.ParseInt(idInput[0], 10, 64)
	if err != nil {
		return 0, "", errors.Annotate(err, "unable to parse buildbucket-id as an integer").Err()
	}

	reasonInput := ctx.Request.Form["reason"]
	if len(reasonInput) != 1 || strings.TrimSpace(reasonInput[0]) == "" {
		return 0, "", fmt.Errorf("invalid or missing reason; expected a non-empty string; actual value: %v", reasonInput)
	}
	reason := strings.TrimSpace(reasonInput[0])

	return id, reason, nil
}

// retryBuildHandler parses inputs from HTTP form, retries the build with the given buildbucket build ID
// then redirects the users to the retry build page.
func retryBuildHandler(ctx *router.Context) error {
	buildbucketID, requestID, err := parseRetryBuildInput(ctx)
	if err != nil {
		return errors.Annotate(err, "error while parsing retry build request input fields").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	build, err := buildbucket.RetryBuild(ctx.Context, buildbucketID, requestID)
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

	buildbucketIDInput := ctx.Request.Form["buildbucket-id"]
	if len(buildbucketIDInput) != 1 || buildbucketIDInput[0] == "" {
		return 0, "", fmt.Errorf("invalid or missing buildbucket-id; expected an integer; actual value: %v", buildbucketIDInput)
	}
	buildbucketID, err = strconv.ParseInt(buildbucketIDInput[0], 10, 64)
	if err != nil {
		return 0, "", errors.Annotate(err, "unable to parse buildbucket-id as an integer").Err()
	}

	retryRequestIDInput := ctx.Request.Form["retry-request-id"]
	if len(retryRequestIDInput) != 1 || strings.TrimSpace(retryRequestIDInput[0]) == "" {
		return 0, "", fmt.Errorf("invalid or missing retry request ID; expected a non-empty string; actual value: %v", retryRequestIDInput)
	}
	retryRequestID = strings.TrimSpace(retryRequestIDInput[0])

	return buildbucketID, retryRequestID, nil
}
