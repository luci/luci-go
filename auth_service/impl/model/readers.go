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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/auth_service/impl/util/gs"
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

// GetAuthorizedEmails returns the emails of all AuthDBReaders.
func GetAuthorizedEmails(ctx context.Context) (stringset.Set, error) {
	// Query for all AuthDBReader entities' keys.
	q := datastore.NewQuery("AuthDBReader").Ancestor(authDBReadersRootKey(ctx)).KeysOnly(true)
	var readerKeys []*datastore.Key
	err := datastore.GetAll(ctx, q, &readerKeys)
	if err != nil {
		return stringset.New(0), errors.Annotate(err, "error getting all AuthDBReader entities").Err()
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
			return errors.Annotate(err, "aborting reader authorization - error getting current readers").Err()
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
		return errors.Annotate(err, "error deauthorizing reader").Err()
	}

	return updateGSReaders(ctx)
}

func updateGSReaders(ctx context.Context) error {
	readers, err := GetAuthorizedEmails(ctx)
	if err != nil {
		return err
	}

	// TODO (b/321137485): Set dry run to false when rolling out AuthDB
	// replication.
	//
	// For now, only set the read ACLs for V2-prefixed objects while
	// still in development & testing.
	dryRun := true
	return gs.UpdateReaders(ctx, readers, dryRun)
}
