// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package policy

import (
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/config/validation"
)

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
	Validate func(cfg ConfigBundle, v *validation.Context)

	// Prepare converts validated configs into an optimized queryable form.
	//
	// This is a user-supplied callback. Must be a pure function.
	//
	// The result of the processing is cached in local instance memory for 1 min.
	// It is supposed to be a read-only object, optimized for performing queries
	// over it.
	//
	// Users of Policy should type-cast it to an appropriate type.
	Prepare func(cfg ConfigBundle, revision string) (Queryable, error)
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
	// a concrete proto message class). May return transient error (e.g timeouts)
	// and fatal ones (e.g bad proto file).
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
	// TODO(vadimsh): Implement.
	return "", nil
}

// Queryable returns a form of the policy document optimized for queries.
//
// This is hot function called from each RPC handler. It uses local in-memory
// cache to store the configs, synchronizing it with the state stored in the
// datastore once a minute.
func (p *Policy) Queryable(c context.Context) (Queryable, error) {
	// TODO(vadimsh): Implement.
	return nil, nil
}
