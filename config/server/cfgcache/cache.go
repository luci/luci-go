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
	"time"

	protov1 "github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	luciproto "go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/caching"
)

// Entry describes what service configuration file to fetch and how to
// deserialize and validate it.
//
// Must be defined as a global variable and registered via Register(...)
// to enable the process cache and the config validation hook.
type Entry struct {
	// Path is a path within the service config set to fetch.
	Path string

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
}

// Register registers the process cache slot and the validation hook.
//
// Must be called during the init time i.e. when initializing a global variable
// or in a package init() function. Get() will panics if used with an
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
	rules.Add(serviceConfigSet, e.Path, func(ctx *validation.Context, _, _ string, content []byte) error {
		_, err := e.validate(ctx, string(content))
		return err
	})

	return e
}

// Update fetches the freshest config and caches it in the datastore.
//
// This is a slow operation. Must be called periodically and asynchronously
// (e.g. from a GAE cron job) to keep the cache fresh.
//
// Performs validation before storing the fetched config.
//
// If `meta` is non-nil, it will receive the config metadata.
func (e *Entry) Update(ctx context.Context, meta *config.Meta) (proto.Message, error) {
	// Fetch the raw text body from LUCI Config service.
	var raw string
	var fetchedMeta config.Meta
	err := cfgclient.Get(ctx, serviceConfigSet, e.Path, cfgclient.String(&raw), &fetchedMeta)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch %s:%s", serviceConfigSet, e.Path).Err()
	}

	// Make sure there are no blocking validation errors. This also deserializes
	// the message.
	valCtx := validation.Context{Context: ctx}
	valCtx.SetFile(e.Path)
	msg, err := e.validate(&valCtx, raw)
	if err != nil {
		return nil, errors.Annotate(err, "failed to perform config validation").Err()
	}
	if err := valCtx.Finalize(); err != nil {
		blocking := err.(*validation.Error).WithSeverity(validation.Blocking)
		if blocking != nil {
			return nil, errors.Annotate(blocking, "validation errors").Err()
		}
	}

	// Reserialize it into a binary proto to make sure older/newer client versions
	// can safely use the proto message when some fields are added/deleted. Text
	// protos do not guarantee that.
	blob, err := proto.Marshal(msg)
	if err != nil {
		return nil, errors.Annotate(err, "failed to reserialize into binary proto").Err()
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
		return nil, errors.Annotate(err, "failed to update the datastore copy").Err()
	}

	if meta != nil {
		*meta = fetchedMeta
	}
	return msg, nil
}

// Get returns the cached config.
//
// Uses in-memory cache to avoid hitting datastore all the time. As a result it
// may keep returning a stale config up to 1 min after the Update call.
//
// Panics if the entry wasn't registered in the process cache via Register.
//
// If `meta` is non-nil, it will receive the config metadata.
func (e *Entry) Get(ctx context.Context, meta *config.Meta) (proto.Message, error) {
	val, err := e.cacheSlot.Fetch(ctx, func(interface{}) (val interface{}, exp time.Duration, err error) {
		pc := procCache{}
		if pc.Config, err = e.Fetch(ctx, &pc.Meta); err != nil {
			return nil, 0, err
		}
		return &pc, time.Minute, nil
	})
	if err != nil {
		return nil, err
	}

	pc := val.(*procCache)
	if meta != nil {
		*meta = pc.Meta
	}
	return pc.Config, nil
}

// Fetch fetches the config from the datastore cache.
//
// Strongly consistent with Update: calling Fetch right after Update will always
// return the most recent config.
//
// Prefer to use Get if possible.
//
// If `meta` is non-nil, it will receive the config metadata.
func (e *Entry) Fetch(ctx context.Context, meta *config.Meta) (proto.Message, error) {
	cached := cachedConfig{ID: e.entityID()}
	if err := datastore.Get(ctx, &cached); err != nil {
		return nil, errors.Annotate(err, "failed to fetch cached config").Err()
	}

	cfg := e.newMessage()
	if err := proto.Unmarshal(cached.Config, cfg); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal cached config").Err()
	}

	if meta != nil {
		*meta = cached.Meta
	}
	return cfg, nil
}

////////////////////////////////////////////////////////////////////////////////

// Note: we can potentially make this configurable if necessary.
const serviceConfigSet = "services/${appid}"

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

// entityID returns an ID to use for cachedConfig entity.
func (e *Entry) entityID() string {
	return fmt.Sprintf("%s:%s", serviceConfigSet, e.Path)
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
