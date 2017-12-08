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

package registration

import (
	"time"

	ds "go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/gcloud/pubsub"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/registration/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/endpoints"
	"go.chromium.org/luci/logdog/common/types"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

func getTopicAndExpiration(c context.Context, req *logdog.RegisterPrefixRequest) (pubsub.Topic, time.Duration, error) {
	// Load our service and project configurations.
	svcs := endpoints.GetServices(c)
	cfg, err := svcs.Config(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to load service configuration.")
		return "", 0, grpcutil.Internal
	}

	pcfg, err := coordinator.CurrentProjectConfig(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to load project configuration.")
		return "", 0, grpcutil.Internal
	}

	// Determine our Pub/Sub topic.
	cfgTransport := cfg.Transport
	if cfgTransport == nil {
		log.Errorf(c, "Missing transport configuration.")
		return "", 0, grpcutil.Internal
	}

	cfgTransportPubSub := cfgTransport.GetPubsub()
	if cfgTransportPubSub == nil {
		log.Errorf(c, "Missing transport Pub/Sub configuration.")
		return "", 0, grpcutil.Internal
	}

	pubsubTopic := pubsub.NewTopic(cfgTransportPubSub.Project, cfgTransportPubSub.Topic)
	if err := pubsubTopic.Validate(); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"topic":      pubsubTopic,
		}.Errorf(c, "Invalid transport Pub/Sub topic.")
		return "", 0, grpcutil.Internal
	}

	expiration := endpoints.MinDuration(req.Expiration, cfg.Coordinator.PrefixExpiration, pcfg.PrefixExpiration)
	return pubsubTopic, expiration, nil
}

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

	pubsubTopic, expiration, err := getTopicAndExpiration(c, req)
	if err != nil {
		log.WithError(err).Errorf(c, "Cannot get pubsubTopic")
		return nil, grpcutil.Errf(codes.Internal, "unable to get pubsubTopic")
	}

	// Determine our prefix expiration. This must be > 0, else there will be no
	// window when log streams can be registered and this prefix is useless.
	//
	// We will choose the shortest expiration window defined by our request and
	// our project and service configurations.
	if expiration <= 0 {
		log.Errorf(c, "Refusing to register prefix in expired state.")
		return nil, grpcutil.Errf(codes.InvalidArgument, "no prefix expiration defined")
	}

	// prep our response with the pubsubTopic
	resp := &logdog.RegisterPrefixResponse{
		LogBundleTopic: string(pubsubTopic),
	}

	// Has the prefix already been registered?
	pfx := &coordinator.LogPrefix{ID: coordinator.LogPrefixID(prefix)}

	// Check for existing prefix registration (non-transactional).
	switch err := ds.Get(c, pfx); err {
	case ds.ErrNoSuchEntity:
		// we'll register it shortly

	case nil:
		if pfx.IsRetry(c, req.OpNonce) {
			log.Infof(c, "The prefix is registered, but we have valid retry (non-transactional).")
			resp.Secret = pfx.Secret
			return resp, nil
		}
		log.Errorf(c, "The prefix is already registered (non-transactional).")
		return nil, grpcutil.AlreadyExists

	default:
		log.WithError(err).Errorf(c, "Failed to check for existing prefix (non-transactional).")
		return nil, grpcutil.Internal
	}

	// The prefix doesn't appear to be registered. Prepare to transactionally
	// register it.
	secret := make(types.PrefixSecret, types.PrefixSecretLength)
	if _, err := cryptorand.Read(c, []byte(secret)); err != nil {
		log.WithError(err).Errorf(c, "Failed to generate prefix secret.")
		return nil, grpcutil.Internal
	}

	pfx.Created = clock.Now(c).UTC()
	pfx.Prefix = string(prefix)
	pfx.Source = req.SourceInfo
	pfx.Secret = []byte(secret)
	pfx.Expiration = pfx.Created.Add(expiration)
	pfx.OpNonce = req.OpNonce
	if err := pfx.Validate(); err != nil {
		log.WithError(err).Errorf(c, "Invalid LogPrefix.")
		return nil, grpcutil.Errf(codes.InvalidArgument, "LogPrefix definition invalid")
	}
	resp.Secret = pfx.Secret

	// Transactionally register the prefix.
	err = ds.RunInTransaction(c, func(c context.Context) error {
		// Get the prefix (if it exists)
		switch err := ds.Get(c, pfx); err {
		case ds.ErrNoSuchEntity:
			// we need to put it

		case nil:
			if pfx.IsRetry(c, req.OpNonce) {
				log.Infof(c, "The prefix is registered, but we have valid retry (transactional).")
				return nil
			}
			log.Errorf(c, "The prefix is already registered (transactional).")
			return grpcutil.AlreadyExists

		default:
			log.WithError(err).Errorf(c, "Failed to check for existing prefix (transactional).")
			return grpcutil.Internal
		}

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

	return resp, nil
}
