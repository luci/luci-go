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
	"fmt"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/auth_service/internal/gs"
)

const (
	// Object ACLs can have at most 100 entries. We limit them to 80 to
	// have some breathing room before the hard limit is reached. When
	// this happens, either some old services should be deauthorized or
	// GCS ACL management should be reimplemented in some different way.
	//
	// See https://cloud.google.com/storage/quotas.
	MaxReaders = 80

	// Limit for an AuthDBReader email, because the maximum size of an
	// indexed string property's UTF-8 encoding in Datastore is 1500 B.
	//
	// See https://cloud.google.com/datastore/docs/concepts/limits.
	MaxReaderEmailLength = 375
)

type AuthDBReader struct {
	// Account that should be able to read AuthDB Google Storage dump.
	//
	// These are accounts that have explicitly requested access to the
	// AuthDB via API call to
	// `/auth_service/api/v1/authdb/subscription/authorization`.
	//
	// They all belong to the `auth-trusted-services` group.
	// Note that we can't just authorize all members of
	// `auth-trusted-services` since we can't enumerate them
	// (for example, there's no way to enumerate glob entries like
	// *@example.com).
	Kind   string         `gae:"$kind,AuthDBReader"`
	Parent *datastore.Key `gae:"$parent"`

	// The account email.
	ID string `gae:"$id"`

	//AuthorizedTS is the time this account was last authorized.
	AuthorizedTS time.Time `gae:"authorized_at,noindex"`
}

func authDBReadersRootKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, "AuthDBReadersRoot", "root", 0, nil)
}

func authDBReaderKey(ctx context.Context, email string) *datastore.Key {
	return datastore.NewKey(ctx, "AuthDBReader", email, 0, authDBReadersRootKey(ctx))
}

func getAllAuthDBReaders(ctx context.Context) ([]*datastore.Key, error) {
	// Query for all AuthDBReader entities' keys.
	q := datastore.NewQuery("AuthDBReader").Ancestor(authDBReadersRootKey(ctx)).KeysOnly(true)
	var readerKeys []*datastore.Key
	err := datastore.GetAll(ctx, q, &readerKeys)
	if err != nil {
		return []*datastore.Key{}, errors.Fmt("error getting all AuthDBReader entities: %w", err)
	}

	return readerKeys, nil
}

// IsAuthorizedReader returns whether the given email is authorized to
// read the AuthDB from Google Storage.
func IsAuthorizedReader(ctx context.Context, email string) (bool, error) {
	reader := &AuthDBReader{
		Kind:   "AuthDBReader",
		Parent: authDBReadersRootKey(ctx),
		ID:     email,
	}
	switch err := datastore.Get(ctx, reader); {
	case err == nil:
		return true, nil
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return false, nil
	default:
		return false, errors.Fmt("error checking AuthDBReader email: %w", err)
	}
}

// GetAuthorizedEmails returns the emails of all AuthDBReaders.
func GetAuthorizedEmails(ctx context.Context) (stringset.Set, error) {
	readerKeys, err := getAllAuthDBReaders(ctx)
	if err != nil {
		return stringset.New(0), err
	}

	// Get the emails from the ID field.
	emails := stringset.New(len(readerKeys))
	for _, key := range readerKeys {
		emails.Add(key.StringID())
	}

	return emails, nil
}

// AuthorizeReader records the given email as an AuthDBReader, then
// updates Auth Service's Google Storage object ACLs.
func AuthorizeReader(ctx context.Context, email string) error {
	if len(email) >= MaxReaderEmailLength {
		return fmt.Errorf("aborting reader authorization - email is too long: %s", email)
	}

	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		readers, err := GetAuthorizedEmails(ctx)
		if err != nil {
			return errors.Fmt("aborting reader authorization - error getting current readers: %w", err)
		}

		if readers.Has(email) {
			// The reader is already authorized; no need to add them.
			return nil
		}

		if len(readers) >= MaxReaders {
			return fmt.Errorf("reached the soft limit on GCS ACL entries")
		}

		// Add the reader.
		reader := &AuthDBReader{
			AuthorizedTS: clock.Now(ctx).UTC(),
		}
		if populated := datastore.PopulateKey(reader, authDBReaderKey(ctx, email)); !populated {
			return errors.New("error authorizing reader - problem setting key")
		}
		return datastore.Put(ctx, reader)
	}, nil)
	if err != nil {
		return err
	}

	return updateGSReaders(ctx)
}

// DeauthorizeReader removes the given email as an AuthDBReader, then
// updates Auth Service's Google Storage object ACLs.
func DeauthorizeReader(ctx context.Context, email string) error {
	err := datastore.Delete(ctx, authDBReaderKey(ctx, email))
	if err != nil {
		return errors.Fmt("error deauthorizing reader: %w", err)
	}

	return updateGSReaders(ctx)
}

// RevokeStaleReaderAccess deletes any AuthDBReaders that are no longer
// in the given trusted group, then updates Auth Service's Google
// Storage object ACLs.
func RevokeStaleReaderAccess(ctx context.Context, trustedGroup string) error {
	s := auth.GetState(ctx)
	if s == nil {
		return fmt.Errorf("error getting AuthDB")
	}
	authDB := s.DB()

	readerKeys, err := getAllAuthDBReaders(ctx)
	if err != nil {
		return err
	}

	toDelete := []*datastore.Key{}
	for _, readerKey := range readerKeys {
		email := readerKey.StringID()
		authIdentity, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, email))
		if err != nil {
			// Non-fatal - just log the error.
			logging.Errorf(ctx, err.Error())
			continue
		}

		trusted, err := authDB.IsMember(ctx, authIdentity, []string{trustedGroup})
		if err != nil {
			return errors.Fmt("error checking %s membership for %s: %w",
				trustedGroup, authIdentity, err)
		}
		if !trusted {
			logging.Warningf(ctx, "stale AuthDB reader access for %s", email)
			toDelete = append(toDelete, readerKey)
		}
	}

	err = datastore.Delete(ctx, toDelete)
	if err != nil {
		return errors.Fmt("error deleting stale AuthDBReaders: %w", err)
	}

	// Update GS ACLs even if no readers were deleted. This is necessary
	// to ensure GS ACLs are updated even if this function errors out
	// while deleting AuthDBReaders - stale readers will be removed on a
	// retry.
	return updateGSReaders(ctx)
}

func updateGSReaders(ctx context.Context) error {
	readers, err := GetAuthorizedEmails(ctx)
	if err != nil {
		return err
	}

	return gs.UpdateReaders(ctx, readers)
}
