// Copyright 2021 The LUCI Authors.
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

// Package model contains datastore model definitions.
package model

import (
	"context"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
)

// AuthVersionedEntityMixin is for AuthDB entities that
// keep track of when they change.
type AuthVersionedEntityMixin struct {
	// ModifiedTS is the time this entity was last modified.
	ModifiedTS time.Time `gae:"modified_ts"`

	// ModifiedBy specifies who mode the modification to this entity.
	ModifiedBy string `gae:"modified_by"`

	// AuthDBRev is the revision number that this entity was last modified at.
	AuthDBRev int `gae:"auth_db_rev"`

	// AuthDBPrevRev is revision number of the previous version of this entity.
	AuthDBPrevRev int `gae:"auth_db_prev_rev"`
}

// AuthGlobalConfig is the root entity for auth models.
// There should be only one instance of this model in datastore.
type AuthGlobalConfig struct {
	// AuthVersionedEntityMixin is embedded
	// to include modification details related to this entity.
	AuthVersionedEntityMixin

	Kind string `gae:"$kind,AuthGlobalConfig"`
	ID   string `gae:"$id,root"`

	// OAuthAdditionalClientIDs are the additional client ID's allowed to access the service.
	OAuthAdditionalClientIDs []string `gae:"oauth_additional_client_ids,noindex"`

	// OAuthClientID is an OAuth2 client_id to use to mint new OAuth2 tokens.
	OAuthClientID string `gae:"oauth_client_id,noindex"`

	// OAuthClientSecret is passed to clients.
	OAuthClientSecret string `gae:"oauth_client_secret,noindex"`

	// TokenServerURL is the URL of the token server to use to generate delegation tokens.
	TokenServerURL string `gae:"token_server_url,noindex"`

	// SecurityConfig is serialized security config from security_config.proto.
	//
	// See https://source.chromium.org/chromium/infra/infra/+/main:luci/appengine/components/components/auth/proto/security_config.proto.
	SecurityConfig []byte `gae:"security_config,noindex"`
}

// AuthGroup is a group of identities, the entity id is the group name.
type AuthGroup struct {
	// AuthVersionedEntityMixin is embedded
	// to include modification details related to this entity.
	AuthVersionedEntityMixin

	Kind string `gae:"$kind,AuthGroup"`
	ID   string `gae:"$id"`

	// Parent is AuthGlobalConfig.
	Parent *datastore.Key `gae:"$parent"`

	// Members is the list of members that are explicitly in this group.
	Members []string `gae:"members"`

	// Globs is the list of identity-glob expressions in this group.
	//
	// Example: ('user@example.com').
	Globs []string `gae:"globs"`

	// Nested is the list of nested group names.
	Nested []string `gae:"nested"`

	// Description is a human readable description of this group.
	Description string `gae:"description,noindex"`

	// Owners is the name of the group that owns this group.
	// Owners can modify or delete this group.
	Owners string `gae:"owners"`

	// CreatedTS is the time when this group was created, in UTC.
	CreatedTS time.Time `gae:"created_ts"`

	// CreatedBy is the email of the user who created this group.
	CreatedBy string `gae:"created_by"`
}

func authGlobalConfigKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, "AuthGlobalConfig", "root", 0, nil)
}

// GetAuthGroup gets the AuthGroup with the given id(groupName).
//
// Returns datastore.ErrNoSuchEntity if the group given is not present.
// Returns an annotated error for other errors.
func GetAuthGroup(ctx context.Context, groupName string) (*AuthGroup, error) {
	authGroup := &AuthGroup{
		ID:     groupName,
		Parent: authGlobalConfigKey(ctx),
	}

	switch err := datastore.Get(ctx, authGroup); {
	case err == nil:
		return authGroup, nil
	case err == datastore.ErrNoSuchEntity:
		return nil, err
	default:
		return nil, errors.Annotate(err, "error getting AuthGroup").Err()
	}
}

// GetAllAuthGroups returs all the AuthGroups from the datastore.
//
// Returns an annotated error.
func GetAllAuthGroups(ctx context.Context) ([]*AuthGroup, error) {
	query := datastore.NewQuery("AuthGroup").Ancestor(authGlobalConfigKey(ctx))
	var authGroups []*AuthGroup
	err := datastore.GetAll(ctx, query, &authGroups)
	if err != nil {
		return nil, errors.Annotate(err, "error getting all AuthGroup entities").Err()
	}
	return authGroups, nil
}
