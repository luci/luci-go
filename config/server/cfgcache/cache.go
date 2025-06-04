// Copyright 2020 The LUCI Authors.
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

// Package cfgcache provides a datastore-based cache of individual config files.
//
// It is intended to be used to cache a small number (usually 1) service
// configuration files fetched from the service's config set and used in every
// request.
//
// Configuration files are assumed to be stored in text protobuf encoding.
package cfgcache

import (
	"context"
	"fmt"
	"sync"
	"time"

	protov1 "github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	luciproto "go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"
)

// Entry describes what service configuration file to fetch and how to
// deserialize and validate it.
//
// Must be defined as a global variable and registered via Register(...)
// to enable the process cache and the config validation hook.
//
// You **must** setup periodical calls to Update to use Get or Fetch. They will
// be returning stale data if Update is not called periodically.
type Entry struct {
	// Path is a path within the service config set to fetch.
	Path string

	// ConfigSet allows to provider custom config set.
	//
	// If empty, defaults to this service's config set.
	ConfigSet string

	// Type identifies proto message type with the config file schema.
	//
	// The actual value will never be used, only its type. All methods will
	// return proto.Message of the exact same type.
	Type proto.Message

	// Validator is called to validate the config correctness.
	//
	// If nil, Update will just validate the config file matches the protobuf
	// message kind identified by Type.
	//
	// If not nil, the Validator will be called with a deserialized message to
	// continue its validation.
	//
	// See go.chromium.org/luci/config/validation for more details.
	Validator func(c *validation.Context, msg proto.Message) error

	// Rules is a config validation ruleset to register the validator in.
	//
	// If nil, the default validation.Rules will be used. This is usually what you
	// want outside of unit tests.
	Rules *validation.RuleSet

	// cacheSlot holds in-process cache of the config.
	cacheSlot caching.SlotHandle

	// See comments for Fetch.
	eagerUpdateOnce sync.Once
	eagerUpdateOK   bool
}

// Register registers the process cache slot and the validation hook.
//
// Must be called during the init time i.e. when initializing a global variable
// or in a package init() function. Get() will panic if used with an
// unregistered entry.
//
// Panics if called twice. Returns `e` itself.
func Register(e *Entry) *Entry {
	if e.cacheSlot.Valid() {
		panic("Register has already been called")
	}
	e.cacheSlot = caching.RegisterCacheSlot()

	rules := e.Rules
	if rules == nil {
		rules = &validation.Rules
	}
	rules.Add(e.configSet(), e.Path, func(ctx *validation.Context, _, _ string, content []byte) error {
		_, err := e.validate(ctx, string(content))
		return err
	})

	return e
}

// Update fetches the freshest config and caches it in the datastore.
//
// **Must** be called periodically and asynchronously (e.g. from a GAE cron job)
// to keep the cache fresh. Get and Fetch will return stale data if Update isn't
// called periodically.
//
// This is a slow operation. Do not put it on hot code paths.
//
// Performs validation before storing the fetched config.
//
// If `meta` is non-nil, it will receive the config metadata.
func (e *Entry) Update(ctx context.Context, meta *config.Meta) (proto.Message, error) {
	// Fetch the raw text body from LUCI Config service.
	var raw string
	var fetchedMeta config.Meta
	err := cfgclient.Get(ctx, config.Set(e.configSet()), e.Path, cfgclient.String(&raw), &fetchedMeta)
	if err != nil {
		return nil, errors.Fmt("failed to fetch %s:%s: %w", e.configSet(), e.Path, err)
	}

	// Make sure there are no blocking validation errors. This also deserializes
	// the message.
	valCtx := validation.Context{Context: ctx}
	valCtx.SetFile(e.Path)
	msg, err := e.validate(&valCtx, raw)
	if err != nil {
		return nil, errors.Fmt("failed to perform config validation: %w", err)
	}
	if err := valCtx.Finalize(); err != nil {
		blocking := err.(*validation.Error).WithSeverity(validation.Blocking)
		if blocking != nil {
			return nil, errors.Fmt("validation errors: %w", blocking)
		}
	}

	// Drop out of any namespaces or transactions.
	ctx = cleanContext(ctx)

	// Quick check if we have it in the datastore already. Useful to skip some
	// transactions if Update is called concurrently by many processes (perhaps
	// through attemptEagerUpdate).
	cur := cachedConfig{ID: e.entityID()}
	if datastore.Get(ctx, &cur); err == nil && cur.Meta.Revision == fetchedMeta.Revision {
		logging.Infof(ctx, "Cached config %s is up-to-date at rev %q", cur.ID, cur.Meta.Revision)
		if meta != nil {
			*meta = fetchedMeta
		}
		return msg, nil
	}

	// Reserialize it into a binary proto to make sure older/newer client versions
	// can safely use the proto message when some fields are added/deleted. Text
	// protos do not guarantee that.
	blob, err := proto.Marshal(msg)
	if err != nil {
		return nil, errors.Fmt("failed to reserialize into binary proto: %w", err)
	}

	// Update the datastore entry if necessary. Do not just unconditionally
	// overwrite it since it will unnecessarily flush memcache dscache entry.
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		cur := cachedConfig{ID: e.entityID()}
		if err := datastore.Get(ctx, &cur); err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}
		if cur.Meta.Revision == fetchedMeta.Revision {
			logging.Infof(ctx, "Cached config %s is up-to-date at rev %q", cur.ID, cur.Meta.Revision)
			return nil
		}
		logging.Infof(ctx, "Updating cached config %s: %q -> %q", cur.ID, cur.Meta.Revision, fetchedMeta.Revision)
		return datastore.Put(ctx, &cachedConfig{
			ID:     cur.ID,
			Config: blob,
			Meta:   fetchedMeta,
		})
	}, nil)
	if err != nil {
		return nil, errors.Fmt("failed to update the datastore copy: %w", err)
	}

	if meta != nil {
		*meta = fetchedMeta
	}
	return msg, nil
}

// Set overrides the cached config in the datastore.
//
// Primarily intended for tests to mock the cached config in the datastore.
func (e *Entry) Set(ctx context.Context, cfg proto.Message, meta *config.Meta) error {
	if cfg.ProtoReflect().Descriptor() != e.Type.ProtoReflect().Descriptor() {
		panic(fmt.Sprintf("got %s, want %s", cfg.ProtoReflect().Descriptor(), e.Type.ProtoReflect().Descriptor()))
	}

	blob, err := proto.Marshal(cfg)
	if err != nil {
		return err
	}

	var m config.Meta
	if meta != nil {
		m = *meta
	}

	return datastore.Put(cleanContext(ctx), &cachedConfig{
		ID:     e.entityID(),
		Config: blob,
		Meta:   m,
	})
}

// Get returns the cached config.
//
// Note: you **must** setup periodical calls to Update to use Get or Fetch. They
// will be returning stale data if Update is not called periodically.
//
// Uses in-memory cache to avoid hitting datastore all the time. As a result it
// may keep returning a stale config up to 1 min after the Update call. Uses
// Fetch to fetch the config if the in-memory cache is stale, see Fetch doc
// to learn its caveats, they apply to Get as well.
//
// If there's no in-memory cache available in the context, falls back to Fetch.
// This happens in tests that don't call caching.WithEmptyProcessCache to setup
// the cache.
//
// Panics if the entry wasn't registered in the process cache via Register.
//
// If `meta` is non-nil, it will receive the config metadata.
func (e *Entry) Get(ctx context.Context, meta *config.Meta) (proto.Message, error) {
	val, err := e.cacheSlot.Fetch(ctx, func(any) (val any, exp time.Duration, err error) {
		pc := procCache{}
		if pc.Config, err = e.Fetch(ctx, &pc.Meta); err != nil {
			return nil, 0, err
		}
		return &pc, time.Minute, nil
	})
	switch {
	case err == caching.ErrNoProcessCache:
		// A fallback useful in unit tests that may not have the process cache
		// available. Production environments usually have the cache installed
		// by the framework code that initializes the root context.
		return e.Fetch(ctx, meta)
	case err != nil:
		return nil, err
	default:
		pc := val.(*procCache)
		if meta != nil {
			*meta = pc.Meta
		}
		return pc.Config, nil
	}
}

// Fetch fetches the config from the datastore cache.
//
// Note: you **must** setup periodical calls to Update to use Get or Fetch. They
// will be returning stale data if Update is not called periodically.
//
// Strongly consistent with Update: calling Fetch right after Update will always
// return the most recent config.
//
// Prefer to use Get if possible.
//
// To simplify deploying code that uses new configs, Fetch will call Update
// itself if it notices there's no cached config in the datastore. To avoid
// overloading LUCI Config, it will do it under the lock and exactly once. If
// this attempt fails, for whatever reason, it will not be retried. Callers of
// Fetch will have to wait until the cache is explicitly updated by Update. If
// you can't risk this situation, deploy code that calls Update before deploying
// code that uses Get or Fetch.
//
// If `meta` is non-nil, it will receive the config metadata.
func (e *Entry) Fetch(ctx context.Context, meta *config.Meta) (proto.Message, error) {
	ctx = cleanContext(ctx)

	cached := cachedConfig{ID: e.entityID()}

	err := datastore.Get(ctx, &cached)
	if err == datastore.ErrNoSuchEntity && e.attemptEagerUpdate(ctx) {
		err = datastore.Get(ctx, &cached)
	}
	if err != nil {
		return nil, errors.Fmt("failed to fetch cached config: %w", err)
	}

	cfg := e.newMessage()
	if err := proto.Unmarshal(cached.Config, cfg); err != nil {
		return nil, errors.Fmt("failed to unmarshal cached config: %w", err)
	}

	if meta != nil {
		*meta = cached.Meta
	}
	return cfg, nil
}

////////////////////////////////////////////////////////////////////////////////

const defaultServiceConfigSet = "services/${appid}"

// cleanContext returns a context with datastore using the default namespace
// and not using transactions.
func cleanContext(ctx context.Context) context.Context {
	return datastore.WithoutTransaction(info.MustNamespace(ctx, ""))
}

// cachedConfig holds binary-serialized config and its metadata.
type cachedConfig struct {
	_kind  string                `gae:"$kind,cfgcache.CachedConfig"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	ID     string      `gae:"$id"`
	Config []byte      `gae:",noindex"`
	Meta   config.Meta `gae:",noindex"`
}

// procCache is stored in the process cache.
type procCache struct {
	Config proto.Message
	Meta   config.Meta
}

// configSet returns overridden ConfigSet or the default.
func (e *Entry) configSet() string {
	if e.ConfigSet != "" {
		return e.ConfigSet
	}
	return defaultServiceConfigSet
}

// entityID returns an ID to use for cachedConfig entity.
func (e *Entry) entityID() string {
	return fmt.Sprintf("%s:%s", e.configSet(), e.Path)
}

// newMessage creates a new empty proto message of e.Type.
func (e *Entry) newMessage() proto.Message {
	return e.Type.ProtoReflect().New().Interface()
}

// validate deserializes the message and passes it through the validator.
func (e *Entry) validate(ctx *validation.Context, content string) (proto.Message, error) {
	msg := e.newMessage()
	if err := luciproto.UnmarshalTextML(content, protov1.MessageV1(msg)); err != nil {
		ctx.Errorf("failed to unmarshal as text proto: %s", err)
		return nil, nil
	}
	if e.Validator != nil {
		if err := e.Validator(ctx, msg); err != nil {
			return nil, err
		}
	}
	return msg, nil
}

// attemptEagerUpdate is called from Fetch if there's no cached config entry.
//
// Will attempt to update the cache exactly once. Returns true if the update
// happened (now or previously) and finished successfully.
func (e *Entry) attemptEagerUpdate(ctx context.Context) bool {
	e.eagerUpdateOnce.Do(func() {
		id := e.entityID()
		meta := config.Meta{}

		logging.Infof(ctx, "Attempting to bootstrap config cache %s", id)
		_, err := e.Update(ctx, &meta)
		if err != nil {
			logging.Errorf(ctx, "Failed to bootstrap config cache %s: %s", id, err)
		} else {
			logging.Infof(ctx, "Successfully bootstrapped config cache %s at rev %s", id, meta.Revision)
		}

		e.eagerUpdateOK = err == nil
	})
	return e.eagerUpdateOK
}
