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
	"context"
	"encoding/json"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/importscfg"
)

type GroupsJSON struct {
	Groups    []string `json:"group_imports"`
	AuthDBRev int64    `json:"auth_db_rev"`
}

func callerEmail(ctx context.Context) (string, error) {
	caller := auth.CurrentIdentity(ctx)
	if caller.Kind() != identity.User {
		return "", status.Error(codes.InvalidArgument, "caller must use email-based auth")
	}

	return caller.Email(), nil
}

// HandleTarballIngestHandler handles the endpoint for ingesting tarballs
// returns a JSON response of the groups that were found in the tarball.
func HandleTarballIngestHandler(ctx *router.Context) error {
	c, r, w := ctx.Request.Context(), ctx.Request, ctx.Writer
	tarballName := ctx.Params.ByName("tarballName")

	email, err := callerEmail(c)
	if err != nil {
		return errors.Fmt("error getting caller email: %w", err)
	}

	// Check if the caller is authorized to upload this tarball.
	importerCfg, _, err := importscfg.Get(c)
	if err != nil {
		logging.Errorf(c, "Transient error loading imports.cfg: %s", err)
		return status.Error(codes.Internal, "transient error checking uploader authorization")
	}
	if !importscfg.IsAuthorizedUploader(importerCfg, email, tarballName) {
		return status.Errorf(codes.PermissionDenied, "unauthorized tarball uploader: %s", email)
	}

	groups, revision, err := model.IngestTarball(c, importerCfg, tarballName, r.Body)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrInvalidTarball):
			// The caller provided bad tarball data.
			return status.Error(codes.InvalidArgument, err.Error())
		default:
			// Log the actual error then only return a generic permission error,
			// to avoid leaking information about the importer config.
			logging.Errorf(c, "Returning PermissionDenied: %s", err)
			return status.Error(codes.PermissionDenied, "unauthorized tarball uploader")
		}
	}

	groupResp, err := json.Marshal(GroupsJSON{
		Groups:    groups,
		AuthDBRev: revision,
	})

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		return errors.Fmt("Error while marshaling JSON: %w", err)
	}

	if _, err = w.Write(groupResp); err != nil {
		return errors.Fmt("Error while writing JSON: %w", err)
	}

	return nil
}
