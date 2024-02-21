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

// Package subscription contains functionality to handle AuthDB access,
// including PubSub subscriptions to AuthDB changes, and ACLs to the
// AuthDB in Google Storage.
package subscription

import (
	"context"
	"encoding/json"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/auth_service/impl/util/gs"
)

type gsAccess struct {
	AuthDBGSPath string `json:"auth_db_gs_path"`
	Authorized   bool   `json:"authorized"`
}

type responseJSON struct {
	// TODO: Add PubSub topic, and PubSub authorization status.
	GS gsAccess `json:"gs"`
}

func callerEmail(ctx context.Context) (string, error) {
	caller := auth.CurrentIdentity(ctx)
	if caller.Kind() != identity.User {
		return "", status.Error(codes.InvalidArgument, "caller must use email-based auth")
	}

	return caller.Email(), nil
}

// CheckAccess queries whether the caller is authorized to access the
// AuthDB already.
//
// Response body:
//
//	{
//		'gs': {
//			'auth_db_gs_path': <same as auth_db_gs_path in SettingsCfg proto>,
//			'authorized': <true if the caller should be able to read GS files>
//		}
//	}
func CheckAccess(ctx *router.Context) error {
	c := ctx.Request.Context()

	email, err := callerEmail(c)
	if err != nil {
		return errors.Annotate(err, "error getting caller email").Err()
	}

	isAuthorized, err := model.IsAuthorizedReader(c, email)
	if err != nil {
		return errors.Annotate(err, "error checking authorization status").Err()
	}

	return respond(ctx, isAuthorized)
}

// Subscribe authorizes the caller to access the AuthDB.
//
// Response body:
//
//	{
//		'gs': {
//			'auth_db_gs_path': <same as auth_db_gs_path in SettingsCfg proto>,
//			'authorized': true
//		}
//	}
func Subscribe(ctx *router.Context) error {
	c := ctx.Request.Context()

	email, err := callerEmail(c)
	if err != nil {
		return errors.Annotate(err, "error getting caller email").Err()
	}

	if err := model.AuthorizeReader(c, email); err != nil {
		return errors.Annotate(err, "error authorizing caller").Err()
	}

	return respond(ctx, true)
}

// Unsubscribe revokes the caller's authorization to access the AuthDB,
// if it existed.
//
// Response body:
//
//	{
//		'gs': {
//			'auth_db_gs_path': <same as auth_db_gs_path in SettingsCfg proto>,
//			'authorized': false
//		}
//	}
func Unsubscribe(ctx *router.Context) error {
	c := ctx.Request.Context()

	email, err := callerEmail(c)
	if err != nil {
		return errors.Annotate(err, "error getting caller email").Err()
	}

	if err := model.DeauthorizeReader(c, email); err != nil {
		return errors.Annotate(err, "error deauthorizing caller").Err()
	}

	return respond(ctx, false)
}

func respond(ctx *router.Context, gsAuthorized bool) error {
	gsPath, err := gs.GetPath(ctx.Request.Context())
	if err != nil {
		return errors.Annotate(err, "error getting GS path from configs").Err()
	}

	response := responseJSON{
		GS: gsAccess{
			AuthDBGSPath: gsPath,
			Authorized:   gsAuthorized,
		},
	}
	blob, err := json.Marshal(response)
	if err != nil {
		return errors.Annotate(err, "error marshalling JSON").Err()
	}

	if _, err = ctx.Writer.Write(blob); err != nil {
		return errors.Annotate(err, "error writing JSON response").Err()
	}

	return nil
}
