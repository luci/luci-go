// Copyright 2016 The LUCI Authors.
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

package registration

import (
	ds "github.com/luci/gae/service/datastore"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/data/rand/cryptorand"
	"github.com/luci/luci-go/common/gcloud/pubsub"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/grpc/grpcutil"
	"github.com/luci/luci-go/logdog/api/endpoints/coordinator/registration/v1"
	"github.com/luci/luci-go/logdog/appengine/coordinator"
	"github.com/luci/luci-go/logdog/appengine/coordinator/endpoints"
	"github.com/luci/luci-go/logdog/appengine/coordinator/hierarchy"
	"github.com/luci/luci-go/logdog/common/types"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

func (s *server) RegisterPrefix(c context.Context, req *logdog.RegisterPrefixRequest) (*logdog.RegisterPrefixResponse, error) {
	log.Fields{
		"project":    req.Project,
		"prefix":     req.Prefix,
		"source":     req.SourceInfo,
		"expiration": google.DurationFromProto(req.Expiration),
	}.Debugf(c, "Registering log prefix.")

	// Confirm that the Prefix is a valid stream name.
	prefix := types.StreamName(req.Prefix)
	if err := prefix.Validate(); err != nil {
		log.WithError(err).Warningf(c, "Invalid prefix.")
		return nil, grpcutil.Errf(codes.InvalidArgument, "invalid prefix")
	}

	// Has the prefix already been registered?
	pfx := &coordinator.LogPrefix{ID: coordinator.LogPrefixID(prefix)}

	// Check for existing prefix registration (non-transactional).
	switch exists, err := ds.Exists(c, ds.KeyForObj(c, pfx)); {
	case err != nil:
		log.WithError(err).Errorf(c, "Failed to check for existing prefix (non-transactional).")
		return nil, grpcutil.Internal

	case exists.All():
		log.Errorf(c, "The prefix is already registered (non-transactional).")
		return nil, grpcutil.AlreadyExists
	}

	// Load our service and project configurations.
	svcs := coordinator.GetServices(c)
	cfg, err := svcs.Config(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to load service configuration.")
		return nil, grpcutil.Internal
	}

	pcfg, err := coordinator.CurrentProjectConfig(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to load project configuration.")
		return nil, grpcutil.Internal
	}

	// Determine our prefix expiration. This must be > 0, else there will be no
	// window when log streams can be registered and this prefix is useless.
	//
	// We will choose the shortest expiration window defined by our request and
	// our project and service configurations.
	expiration := endpoints.MinDuration(req.Expiration, cfg.Coordinator.PrefixExpiration, pcfg.PrefixExpiration)
	if expiration <= 0 {
		log.Errorf(c, "Refusing to register prefix in expired state.")
		return nil, grpcutil.Errf(codes.InvalidArgument, "no prefix expiration defined")
	}

	// Determine our Pub/Sub topic.
	cfgTransport := cfg.Transport
	if cfgTransport == nil {
		log.Errorf(c, "Missing transport configuration.")
		return nil, grpcutil.Internal
	}

	cfgTransportPubSub := cfgTransport.GetPubsub()
	if cfgTransportPubSub == nil {
		log.Errorf(c, "Missing transport Pub/Sub configuration.")
		return nil, grpcutil.Internal
	}

	pubsubTopic := pubsub.NewTopic(cfgTransportPubSub.Project, cfgTransportPubSub.Topic)
	if err := pubsubTopic.Validate(); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"topic":      pubsubTopic,
		}.Errorf(c, "Invalid transport Pub/Sub topic.")
		return nil, grpcutil.Internal
	}

	// Best effort: register the stream prefix hierarchy components, including the
	// separator.
	//
	// Determine which hierarchy components we need to add.
	comps := hierarchy.Components(prefix.AsPathPrefix(""), false)
	if comps, err = hierarchy.Missing(c, comps); err != nil {
		log.WithError(err).Warningf(c, "Failed to probe for missing hierarchy components.")
	}

	// Before we go into transaction, try and put these entries. This should not
	// be contested, since components don't share an entity root.
	//
	// If this fails, that's okay; we'll handle this when the stream gets
	// registered.
	if err := hierarchy.PutMulti(c, comps); err != nil {
		log.WithError(err).Infof(c, "Failed to add missing hierarchy components.")
	}

	// The prefix doesn't appear to be registered. Prepare to transactionally
	// register it.
	now := clock.Now(c).UTC()

	// Generate a prefix secret.
	secret := make(types.PrefixSecret, types.PrefixSecretLength)
	if _, err := cryptorand.Read(c, []byte(secret)); err != nil {
		log.WithError(err).Errorf(c, "Failed to generate prefix secret.")
		return nil, grpcutil.Internal
	}
	if err := secret.Validate(); err != nil {
		log.WithError(err).Errorf(c, "Generated invalid prefix secret.")
		return nil, grpcutil.Internal
	}

	// Transactionally register the prefix.
	err = ds.RunInTransaction(c, func(c context.Context) error {
		// Check if this Prefix exists (transactional).
		switch exists, err := ds.Exists(c, ds.KeyForObj(c, pfx)); {
		case err != nil:
			log.WithError(err).Errorf(c, "Failed to check for existing prefix (transactional).")
			return grpcutil.Internal

		case exists.All():
			log.Errorf(c, "The prefix is already registered (transactional).")
			return grpcutil.AlreadyExists
		}

		// The Prefix is not registered, so let's register it.
		pfx.Created = now
		pfx.Prefix = string(prefix)
		pfx.Source = req.SourceInfo
		pfx.Secret = []byte(secret)
		pfx.Expiration = now.Add(expiration)

		if err := ds.Put(c, pfx); err != nil {
			log.WithError(err).Errorf(c, "Failed to register prefix.")
			return grpcutil.Internal
		}

		log.Infof(c, "The prefix was successfully registered.")
		return nil
	}, nil)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to register prefix (transactional).")
		return nil, err
	}

	return &logdog.RegisterPrefixResponse{
		Secret:         []byte(secret),
		LogBundleTopic: string(pubsubTopic),
	}, nil
}
