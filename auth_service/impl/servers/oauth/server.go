// Copyright 2022 The LUCI Authors.
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

// Package oauth contains methods to work with oauth endpoint.
package oauth

import (
	"encoding/json"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/auth_service/impl/model"
)

// HandleLegacyOAuthEndpoint returns client_id and client_secret to support
// legacy services that depend on the legacy oauth endpoint for OAuth2 login on a client.
// Returns client_id and client_secret to use for OAuth2 login on a client.
func HandleLegacyOAuthEndpoint(ctx *router.Context) error {
	c, w := ctx.Request.Context(), ctx.Writer
	var globalCfgEntity *model.AuthGlobalConfig
	var replicationStateEntity *model.AuthReplicationState
	var err error

	switch globalCfgEntity, err = model.GetAuthGlobalConfig(c); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		errors.Log(c, err)
		return status.Errorf(codes.Internal, "no Global Config entity found in datastore.")
	case err != nil:
		errors.Log(c, err)
		return status.Errorf(codes.Internal, "something went wrong... see logs")
	}

	switch replicationStateEntity, err = model.GetReplicationState(c); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		errors.Log(c, err)
		return status.Errorf(codes.Internal, "no Replication State entity found in datastore.")
	case err != nil:
		errors.Log(c, err)
		return status.Errorf(codes.Internal, "something went wrong... see logs")
	}

	blob, err := json.Marshal(map[string]any{
		"token_server_url":      globalCfgEntity.TokenServerURL,
		"client_not_so_secret":  globalCfgEntity.OAuthClientSecret,
		"additional_client_ids": globalCfgEntity.OAuthAdditionalClientIDs,
		"client_id":             globalCfgEntity.OAuthClientID,
		"primary_url":           replicationStateEntity.PrimaryURL,
	})

	if err != nil {
		return err
	}

	if _, err := w.Write(blob); err != nil {
		return err
	}

	return nil
}
