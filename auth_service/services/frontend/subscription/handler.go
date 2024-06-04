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
// including authorization to subscribe to the Pubsub topic for AuthDB
// change notifications, and updating ACLs to the AuthDB in Google Cloud
// Storage.
package subscription

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
	"go.chromium.org/luci/auth_service/internal/gs"
	"go.chromium.org/luci/auth_service/internal/pubsub"
)

type gsAccess struct {
	AuthDBGSPath string `json:"auth_db_gs_path"`
	Authorized   bool   `json:"authorized"`
}

type responseJSON struct {
	PubsubTopic      string   `json:"topic"`
	PubsubAuthorized bool     `json:"authorized"`
	GS               gsAccess `json:"gs"`
}

func callerEmail(ctx context.Context) (string, error) {
	caller := auth.CurrentIdentity(ctx)
	if caller.Kind() != identity.User {
		return "", status.Error(codes.InvalidArgument, "caller must use email-based auth")
	}

	return caller.Email(), nil
}

// CheckAccess queries whether the caller is authorized to:
// - subscribe to AuthDB change notifications from Pubsub; and
// - read the AuthDB from Google Cloud Storage.
//
// Response body:
//
//	{
//		'topic': <full name of Pubsub topic for AuthDB change notifications>,
//		'authorized': <true if the caller is allowed to subscribe to it>,
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

	psAuthorized, err := pubsub.IsAuthorizedSubscriber(c, email)
	if err != nil {
		return errors.Annotate(err, "error checking Pubsub subscription").Err()
	}

	gsAuthorized, err := model.IsAuthorizedReader(c, email)
	if err != nil {
		return errors.Annotate(err, "error checking authorization status").Err()
	}

	return respond(ctx, psAuthorized, gsAuthorized)
}

// Authorize authorizes the caller to:
// - subscribe to AuthDB change notifications from Pubsub; and
// - read the AuthDB from Google Cloud Storage.
//
// Response body:
//
//	{
//		'topic': <full name of Pubsub topic for AuthDB change notifications>,
//		'authorized': true,
//		'gs': {
//			'auth_db_gs_path': <same as auth_db_gs_path in SettingsCfg proto>,
//			'authorized': true
//		}
//	}
func Authorize(ctx *router.Context) error {
	c := ctx.Request.Context()

	email, err := callerEmail(c)
	if err != nil {
		return errors.Annotate(err, "error getting caller email").Err()
	}

	eligible, err := auth.IsMember(c, model.TrustedServicesGroup, model.AdminGroup)
	if err != nil {
		err = errors.Annotate(err, "error checking subscribing eligibility for %s", email).Err()
		logging.Errorf(c, err.Error())
		return status.Error(codes.Internal, "error checking caller subscribing eligibility")
	}
	if !eligible {
		return status.Errorf(codes.PermissionDenied, "caller is ineligible to subscribe")
	}

	if err := pubsub.AuthorizeSubscriber(c, email); err != nil {
		return errors.Annotate(err, "error granting Pubsub subscriber role").Err()
	}

	if err := model.AuthorizeReader(c, email); err != nil {
		return errors.Annotate(err, "error granting Google Storage read access").Err()
	}

	return respond(ctx, true, true)
}

// Deauthorize revokes the caller's authorization to:
// - subscribe to AuthDB change notifications from Pubsub; and
// - read the AuthDB from Google Cloud Storage.
//
// Response body:
//
//	{
//		'topic': <full name of Pubsub topic for AuthDB change notifications>,
//		'authorized': false,
//		'gs': {
//			'auth_db_gs_path': <same as auth_db_gs_path in SettingsCfg proto>,
//			'authorized': false
//		}
//	}
func Deauthorize(ctx *router.Context) error {
	c := ctx.Request.Context()

	email, err := callerEmail(c)
	if err != nil {
		return errors.Annotate(err, "error getting caller email").Err()
	}

	if err := pubsub.DeauthorizeSubscriber(c, email); err != nil {
		return errors.Annotate(err, "error revoking Pubsub subscriber role").Err()
	}

	if err := model.DeauthorizeReader(c, email); err != nil {
		return errors.Annotate(err, "error revoking Google Storage read access").Err()
	}

	return respond(ctx, false, false)
}

func respond(ctx *router.Context, psAuthorized, gsAuthorized bool) error {
	gsPath, err := gs.GetPath(ctx.Request.Context())
	if err != nil {
		return errors.Annotate(err, "error getting GS path from configs").Err()
	}

	topic := pubsub.GetAuthDBChangeTopic(ctx.Request.Context())
	response := responseJSON{
		PubsubTopic:      topic,
		PubsubAuthorized: psAuthorized,
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
