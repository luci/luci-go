// Copyright 2015 The LUCI Authors.
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

package coordinator

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	ds "go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/logdog/common/types"
)

const (
	// RegistrationNonceTimeout is how long LogPrefix.IsRetry will consider
	// a matching nonce to be valid.
	RegistrationNonceTimeout = 15 * time.Minute
)

// LogPrefix is a datastore model for a prefix space. All log streams sharing
// a prefix will have a LogPrefix entry to group under.
//
// A LogPrefix is keyed on the hash of its Prefix property.
//
// Prefix-scoped properties are used to control creation and modification
// attributes of log streams sharing the prefix.
type LogPrefix struct {
	// ID is the LogPrefix's ID. It is an encoded hash value generated from the
	// stream's Prefix field.
	ID HashID `gae:"$id"`

	// Schema is the datastore schema version for this object. This can be used
	// to facilitate schema migrations.
	//
	// The current schema is currentSchemaVersion.
	Schema string

	// Created is the time when this stream was created.
	Created time.Time `gae:",noindex"`

	// Prefix is this log stream's prefix value. Log streams with the same prefix
	// are logically grouped.
	//
	// This value should not be changed once populated, as it will invalidate the
	// HashID.
	Prefix string `gae:",noindex"`

	// Realm is a full realm name ("<project>:<realm>") with ACLs for this prefix.
	//
	// It is set in RegisterStream and can't be changed afterwards.
	Realm string

	// Source is the (indexed) set of source strings sent by the prefix registrar.
	Source []string

	// Expiration is the time when this log prefix expires. Stream registrations
	// for this prefix will fail after this point.
	Expiration time.Time

	// Secret is the Butler secret value for this prefix. All streams within
	// the prefix share this secret value.
	//
	// This value may only be returned to LogDog services; it is not user-visible.
	Secret []byte `gae:",noindex"`

	// OpNonce is provided by the client when calling RegisterPrefix. If the
	// client provides the same nonce on a subsequent invocation of
	// RegisterPrefix, the server will respond with success instead of
	// AlreadyExists.
	//
	// This must have a length of either 0 or types.OpNonceLength.
	//
	// The nonce has a valid lifetime of RegistrationNonceTimeout after Created.
	OpNonce []byte `gae:",noindex"`

	// extra causes datastore to ignore unrecognized fields and strip them in
	// future writes.
	extra ds.PropertyMap `gae:"-,extra"`
}

var _ interface {
	ds.PropertyLoadSaver
} = (*LogPrefix)(nil)

// LogPrefixID returns the HashID for a specific prefix.
func LogPrefixID(prefix types.StreamName) HashID {
	return makeHashID(string(prefix))
}

// Load implements ds.PropertyLoadSaver.
func (p *LogPrefix) Load(pmap ds.PropertyMap) error {
	if err := ds.GetPLS(p).Load(pmap); err != nil {
		return err
	}

	// Validate the log prefix. Don't enforce HashID correctness, since datastore
	// hasn't populated that field yet.
	return p.validateImpl(false)
}

// Save implements ds.PropertyLoadSaver.
func (p *LogPrefix) Save(withMeta bool) (ds.PropertyMap, error) {
	if err := p.validateImpl(true); err != nil {
		return nil, err
	}
	p.Schema = CurrentSchemaVersion

	return ds.GetPLS(p).Save(withMeta)
}

// IsRetry checks to see if this LogPrefix is still in the OpNonce
// window, and if nonce matches the one in this LogPrefix.
func (p *LogPrefix) IsRetry(c context.Context, nonce []byte) bool {
	if len(nonce) != types.OpNonceLength {
		logging.Infof(c, "user provided invalid nonce length (%d)", len(nonce))
		return false
	}
	if len(p.OpNonce) == 0 {
		logging.Infof(c, "prefix %q has no associated nonce", p.Prefix)
		return false
	}
	if clock.Now(c).After(p.Created.Add(RegistrationNonceTimeout)) {
		logging.Infof(c, "prefix %q has expired nonce", p.Prefix)
		return false
	}
	return subtle.ConstantTimeCompare(p.OpNonce, nonce) == 1
}

// getIDFromPrefix calculates the log stream's hash ID from its Prefix/Name
// fields, which must be populated else this function will panic.
func (p *LogPrefix) getIDFromPrefix() HashID {
	if p.Prefix == "" {
		panic("empty prefix")
	}
	return makeHashID(p.Prefix)
}

// Validate evaluates the state and data contents of the LogPrefix and returns
// an error if it is invalid.
func (p *LogPrefix) Validate() error {
	return p.validateImpl(true)
}

func (p *LogPrefix) validateImpl(enforceHashID bool) error {
	if enforceHashID {
		// Make sure our Prefix and Name match the Hash ID.
		if hid := p.getIDFromPrefix(); hid != p.ID {
			return fmt.Errorf("hash IDs don't match (%q != %q)", hid, p.ID)
		}
	}

	if err := types.StreamName(p.Prefix).Validate(); err != nil {
		return fmt.Errorf("invalid prefix: %s", err)
	}
	if err := types.PrefixSecret(p.Secret).Validate(); err != nil {
		return fmt.Errorf("invalid prefix secret: %s", err)
	}
	if p.Created.IsZero() {
		return errors.New("created time is not set")
	}
	if l := len(p.OpNonce); l > 0 && l != types.OpNonceLength {
		return fmt.Errorf("registration nonce has bad length (%d)", l)
	}
	return nil
}
