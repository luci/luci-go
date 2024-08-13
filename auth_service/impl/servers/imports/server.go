// Copyright 2023 The LUCI Authors.
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

// Package imports contains Imports endpoints implementation.
package imports

import (
	"encoding/json"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/auth_service/impl/model"
)

type GroupsJSON struct {
	Groups    []string `json:"group_imports"`
	AuthDBRev int64    `json:"auth_db_rev"`
}

// HandleTarballIngestHandler handles the endpoint for ingesting tarballs
// returns a JSON response of the groups that were found in the tarball.
func HandleTarballIngestHandler(ctx *router.Context) error {
	c, r, w := ctx.Request.Context(), ctx.Request, ctx.Writer
	var err error
	tarballName := ctx.Params.ByName("tarballName")
	groups, revision, err := model.IngestTarball(c, tarballName, r.Body)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrInvalidTarball):
			// Loading the tarball is only attempted once the caller has been
			// verified as an authorized uploader, so we can return the actual
			// error message.
			return status.Errorf(codes.InvalidArgument, err.Error())
		default:
			// Log the actual error then only return a generic permission error,
			// to avoid leaking information about the importer config.
			logging.Errorf(c, "%w", err)
			err = model.ErrUnauthorizedUploader
			return status.Errorf(codes.PermissionDenied, err.Error())
		}
	}

	groupResp, err := json.Marshal(GroupsJSON{
		Groups:    groups,
		AuthDBRev: revision,
	})

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		return errors.Annotate(err, "Error while marshaling JSON").Err()
	}

	if _, err = w.Write(groupResp); err != nil {
		return errors.Annotate(err, "Error while writing JSON").Err()
	}

	return nil
}
