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

package model

import (
	"context"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tokens"
)

// legacyBootstrapToken described how to generate bootstrap tokens.
//
// TODO(b/355013257): Get rid of this and switch to Tink secrets once Python
// code is no longer in use.
var legacyBootstrapToken = tokens.TokenKind{
	Algo:       tokens.TokenAlgoHmacSHA256,
	Expiration: time.Hour,
	SecretKey:  "bot_bootstrap_token",
	Version:    1,
}

// cachedSecret is used to cache the legacy bootstrap secret key in the process
// memory to avoid fetching it all the time (it never changes).
var cachedSecret = caching.RegisterCacheSlot()

// LegacyBootstrapSecret is a subset of the AuthSecret python entity with the
// bootstrap token secret.
type LegacyBootstrapSecret struct {
	_ datastore.PropertyMap `gae:"-,extra"`

	// Key should be LegacyBootstrapSecretKey(...).
	Key *datastore.Key `gae:"$key"`
	// Values is a list of historical values of the secret.
	Values [][]byte `gae:"values,noindex"`
}

// LegacyBootstrapSecretKey is a datastore key of the bootstrap secret.
func LegacyBootstrapSecretKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, "AuthSecret", legacyBootstrapToken.SecretKey, 0,
		datastore.NewKey(ctx, "AuthSecretScope", "local", 0,
			datastore.NewKey(ctx, "AuthGlobalConfig", "root", 0, nil),
		),
	)
}

// GenerateBootstrapToken generates a token used to authenticate bot code fetch
// requests used during the bot bootstrap.
func GenerateBootstrapToken(ctx context.Context, caller identity.Identity) (string, error) {
	return legacyBootstrapToken.Generate(useLegacyStore(ctx), nil, map[string]string{
		"for": string(caller),
	}, 0)
}

// ValidateBootstrapToken checks if the bootstrap token is valid.
//
// If the token is valid returns the identity of whoever generated it. Returns
// a fatal error if the token is invalid or a transient error if the validation
// process itself failed (e.g. a datastore error).
func ValidateBootstrapToken(ctx context.Context, tok string) (identity.Identity, error) {
	payload, err := legacyBootstrapToken.Validate(useLegacyStore(ctx), tok, nil)
	if err != nil {
		return "", err
	}
	ident, err := identity.MakeIdentity(payload["for"])
	if err != nil {
		return "", errors.Annotate(err, "bad token payload %v", payload).Err()
	}
	return ident, nil
}

// useLegacyStore puts into the context a secrets.Store that knows how to load
// Python secrets for backward compatibility.
//
// TODO(b/355013257): Get rid of this and switch to Tink secrets once Python
// code is no longer in use.
func useLegacyStore(ctx context.Context) context.Context {
	return secrets.Use(ctx, legacyStore{})
}

// legacyStore implements secrets.Store.
type legacyStore struct{}

// RandomSecret returns a random secret given its name.
//
// Supports only "bot_bootstrap_token" secret. Loads it from the legacy Python
// entity.
func (legacyStore) RandomSecret(ctx context.Context, name string) (secrets.Secret, error) {
	if name != legacyBootstrapToken.SecretKey {
		return secrets.Secret{}, errors.Reason("unexpected key requested: %s", name).Err()
	}
	blob, err := cachedSecret.Fetch(ctx, func(any) (blob any, exp time.Duration, err error) {
		ent := &LegacyBootstrapSecret{Key: LegacyBootstrapSecretKey(ctx)}
		if err = datastore.Get(ctx, ent); err != nil {
			return nil, 0, err
		}
		if len(ent.Values) == 0 {
			return nil, 0, errors.New("no secret values in the entity")
		}
		return ent.Values[0], 24 * time.Hour, nil
	})
	if err != nil {
		return secrets.Secret{}, transient.Tag.Apply(err)
	}
	return secrets.Secret{Active: blob.([]byte)}, nil
}

// StoredSecret returns a previously stored secret given its name.
func (legacyStore) StoredSecret(ctx context.Context, name string) (secrets.Secret, error) {
	return secrets.Secret{}, errors.New("not implemented and should not be called")
}

// AddRotationHandler registers a callback called when the secret is updated.
func (legacyStore) AddRotationHandler(ctx context.Context, name string, cb secrets.RotationHandler) error {
	return errors.New("not implemented and should not be called")
}
