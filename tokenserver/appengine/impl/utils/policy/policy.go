// Copyright 2017 The LUCI Authors.
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

package policy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/caching/lazyslot"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config/validation"
)

// ErrNoPolicy is returned by Queryable(...) if a policy is not yet available.
//
// This happens when the service is deployed for the first time and policy
// configs aren't fetched yet. This error will not show up if ImportConfigs
// succeeded at least once.
var ErrNoPolicy = errors.New("policy config is not imported yet")

// Policy describes how to fetch, store and parse policy documents.
//
// This is a singleton-like object that should be shared by multiple requests.
//
// Each instance corresponds to one kind of a policy and it keeps a Queryable
// form if the corresponding policy cached in local memory, occasionally
// updating it based on the configs stored in the datastore (that are in turn
// periodically updated from a cron).
type Policy struct {
	// Name defines the name of the policy, e.g. "delegation rules".
	//
	// It is used in datastore IDs and for logging.
	Name string

	// Fetch fetches and parses all relevant text proto files.
	//
	// This is a user-supplied callback.
	//
	// Called from cron when ingesting new configs. It must return either a non
	// empty bundle with configs or an error.
	Fetch func(c context.Context, f ConfigFetcher) (ConfigBundle, error)

	// Validate verifies the fetched config files are semantically valid.
	//
	// This is a user-supplied callback. Must be a pure function.
	//
	// Reports all errors through the given validation.Context object. The config
	// is considered valid if there are no errors reported. A valid config must be
	// accepted by Prepare without errors.
	//
	// Called from cron when ingesting new configs.
	Validate func(v *validation.Context, cfg ConfigBundle)

	// Prepare converts validated configs into an optimized queryable form.
	//
	// This is a user-supplied callback. Must be a pure function.
	//
	// The result of the processing is cached in local instance memory for 1 min.
	// It is supposed to be a read-only object, optimized for performing queries
	// over it.
	//
	// Users of Policy should type-cast it to an appropriate type.
	Prepare func(c context.Context, cfg ConfigBundle, revision string) (Queryable, error)

	cache lazyslot.Slot // holds and updates in-memory cache of Queryable
}

// Queryable is validated and parsed configs in a form optimized for queries.
//
// This object is shared between multiple requests and kept in memory for as
// long as it still matches the current config.
type Queryable interface {
	// ConfigRevision returns the revision passed to Policy.Prepare.
	//
	// It is a revision of configs used to construct this object. Used for
	// logging.
	ConfigRevision() string
}

// ConfigFetcher hides details of interaction with LUCI Config.
//
// Passed to Fetch callback.
type ConfigFetcher interface {
	// FetchTextProto fetches text-serialized protobuf message at a given path.
	//
	// The path is relative to the token server config set root in LUCI config.
	//
	// On success returns nil and fills in 'out' (which should be a pointer to
	// a concrete proto message class). May return transient error (e.g. timeouts)
	// and fatal ones (e.g. bad proto file).
	FetchTextProto(c context.Context, path string, out proto.Message) error
}

// ImportConfigs updates configs stored in the datastore.
//
// Is should be periodically called from a cron.
//
// Returns the revision of the configs that are now in the datastore. It's
// either the imported revision, if configs change, or a previously known
// revision, if configs at HEAD are same.
//
// Validation errors are returned as *validation.Error struct. Use type cast to
// sniff them, if necessary.
func (p *Policy) ImportConfigs(c context.Context) (rev string, err error) {
	c = logging.SetField(c, "policy", p.Name)

	// Fetch and parse text protos stored in LUCI config. The fetcher will also
	// record the revision of the fetched files.
	fetcher := luciConfigFetcher{}
	bundle, err := p.Fetch(c, &fetcher)
	if err == nil && len(bundle) == 0 {
		err = errors.New("no configs fetched by the callback")
	}
	if err != nil {
		return "", errors.Fmt("failed to fetch policy configs: %w", err)
	}
	rev = fetcher.Revision()

	// Convert configs into a form appropriate for the datastore. We'll skip the
	// rest of the import if this exact blob is already in the datastore (based on
	// SHA256 digest).
	cfgBlob, err := serializeBundle(bundle)
	if err != nil {
		return "", errors.Fmt("failed to serialize the configs: %w", err)
	}
	digest := sha256.Sum256(cfgBlob)
	digestHex := hex.EncodeToString(digest[:])
	logging.Infof(c, "Got %d bytes of configs (SHA256 %s)", len(cfgBlob), digestHex)

	// Do we have it already?
	existingHdr, err := getImportedPolicyHeader(c, p.Name)
	if err != nil {
		return "", errors.Fmt("failed to grab ImportedPolicyHeader: %w", err)
	}
	if existingHdr != nil && digestHex == existingHdr.SHA256 {
		logging.Infof(
			c, "Configs are up-to-date. Last changed at rev %s, last checked rev is %s.",
			existingHdr.Revision, rev)
		return existingHdr.Revision, nil
	}

	existingRev := "(nil)"
	if existingHdr != nil {
		existingRev = existingHdr.Revision
	}
	logging.Infof(c, "Policy config changed: %s -> %s", existingRev, rev)

	if p.Validate != nil {
		ctx := &validation.Context{Context: c}
		p.Validate(ctx, bundle)
		if err := ctx.Finalize(); err != nil {
			return "", errors.Fmt("configs at rev %s are invalid: %w", rev, err)
		}
	}

	// Double check that they actually can be parsed into a queryable form. If
	// not, the Policy callbacks are buggy.
	queriable, err := p.Prepare(c, bundle, rev)
	if err == nil && queriable.ConfigRevision() != rev {
		err = errors.New("wrong revision in result of Prepare callback")
	}
	if err != nil {
		return "", errors.Fmt("failed to convert configs into a queryable form: %w", err)
	}

	logging.Infof(c, "Storing new configs")
	if err := updateImportedPolicy(c, p.Name, rev, digestHex, cfgBlob); err != nil {
		return "", err
	}

	return rev, nil
}

// Queryable returns a form of the policy document optimized for queries.
//
// This is hot function called from each RPC handler. It uses local in-memory
// cache to store the configs, synchronizing it with the state stored in the
// datastore once a minute.
//
// Returns ErrNoPolicy if the policy config wasn't imported yet.
func (p *Policy) Queryable(c context.Context) (Queryable, error) {
	val, err := p.cache.Get(c, func(prev any) (newQ any, exp time.Duration, err error) {
		prevQ, _ := prev.(Queryable)
		newQ, err = p.grabQueryable(c, prevQ)
		if err == nil {
			exp = cacheExpiry(c)
		}
		return
	})
	if err != nil {
		return nil, err
	}
	return val.(Queryable), nil
}

// grabQueryable is called whenever cached Queryable in p.cache expires.
func (p *Policy) grabQueryable(c context.Context, prevQ Queryable) (Queryable, error) {
	c = logging.SetField(c, "policy", p.Name)

	logging.Infof(c, "Checking version of the policy in the datastore")
	hdr, err := getImportedPolicyHeader(c, p.Name)
	switch {
	case err != nil:
		return nil, errors.Fmt("failed to fetch importedPolicyHeader entity: %w", err)
	case hdr == nil:
		return nil, ErrNoPolicy
	}

	// Reuse existing Queryable object if configs didn't change.
	if prevQ != nil && prevQ.ConfigRevision() == hdr.Revision {
		return prevQ, nil
	}

	// Fetch new configs.
	logging.Infof(c, "Fetching policy configs from the datastore")
	body, err := getImportedPolicyBody(c, p.Name)
	switch {
	case err != nil:
		return nil, errors.Fmt("failed to fetch importedPolicyBody entity: %w", err)
	case body == nil: // this is rare, the body shouldn't disappear
		logging.Errorf(c, "The policy body is unexpectedly gone")
		return nil, ErrNoPolicy
	}

	// An error here and below can happen if previously validated config is no
	// longer valid (e.g. if the service code is updated and new code doesn't like
	// the stored config anymore).
	//
	// If this check fails, the service is effectively offline until configs are
	// updated. Presumably, it is better than silently using no longer valid
	// config.
	logging.Infof(c, "Using configs at rev %s", body.Revision)
	configs, unknown, err := deserializeBundle(body.Data)
	if err != nil {
		return nil, errors.Fmt("failed to deserialize cached configs: %w", err)
	}
	if len(unknown) != 0 {
		for _, cfg := range unknown {
			logging.Errorf(c, "Unknown proto type %q in cached config %q", cfg.Kind, cfg.Path)
		}
		return nil, errors.New("failed to deserialize some cached configs")
	}
	queryable, err := p.Prepare(c, configs, body.Revision)
	if err != nil {
		return nil, errors.Fmt("failed to process cached configs: %w", err)
	}

	return queryable, nil
}

// cacheExpiry returns a random duration from [4 min, 5 min).
//
// It's used to define when to refresh in-memory Queryable cache. We randomize
// it to desynchronize cache updates of different Policy instances.
func cacheExpiry(c context.Context) time.Duration {
	rnd := time.Duration(mathrand.Int63n(c, int64(time.Minute)))
	return 4*time.Minute + rnd
}
