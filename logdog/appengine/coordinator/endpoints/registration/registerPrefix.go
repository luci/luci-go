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
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/gcloud/pubsub"
	log "go.chromium.org/luci/common/logging"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/realms"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/registration/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/endpoints"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/logdog/server/config"
)

func getTopicAndExpiration(c context.Context, req *logdog.RegisterPrefixRequest) (pubsub.Topic, time.Duration, error) {
	// Load our service and project configurations.
	cfg, err := config.Config(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to load service configuration.")
		return "", 0, status.Error(codes.Internal, "internal server error")
	}

	pcfg, err := coordinator.ProjectConfig(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to load project configuration.")
		return "", 0, status.Error(codes.Internal, "internal server error")
	}

	// Determine our Pub/Sub topic.
	cfgTransport := cfg.Transport
	if cfgTransport == nil {
		log.Errorf(c, "Missing transport configuration.")
		return "", 0, status.Error(codes.Internal, "internal server error")
	}

	cfgTransportPubSub := cfgTransport.GetPubsub()
	if cfgTransportPubSub == nil {
		log.Errorf(c, "Missing transport Pub/Sub configuration.")
		return "", 0, status.Error(codes.Internal, "internal server error")
	}

	pubsubTopic := pubsub.NewTopic(cfgTransportPubSub.Project, cfgTransportPubSub.Topic)
	if err := pubsubTopic.Validate(); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"topic":      pubsubTopic,
		}.Errorf(c, "Invalid transport Pub/Sub topic.")
		return "", 0, status.Error(codes.Internal, "internal server error")
	}

	expiration := endpoints.MinDuration(req.Expiration, cfg.Coordinator.PrefixExpiration, pcfg.PrefixExpiration)
	return pubsubTopic, expiration, nil
}

func (s *server) RegisterPrefix(c context.Context, req *logdog.RegisterPrefixRequest) (*logdog.RegisterPrefixResponse, error) {
	log.Fields{
		"project":    req.Project,
		"realm":      req.Realm,
		"prefix":     req.Prefix,
		"source":     req.SourceInfo,
		"expiration": req.Expiration.AsDuration(),
	}.Debugf(c, "Registering log prefix.")

	// Legacy clients do not pass `realm`. This is fine, fallback to "@legacy"
	// realm.
	var realm string
	if req.Realm != "" {
		if err := realms.ValidateRealmName(req.Realm, realms.ProjectScope); err != nil {
			log.WithError(err).Warningf(c, "Invalid realm.")
			return nil, status.Errorf(codes.InvalidArgument, "%s", err)
		}
		realm = realms.Join(req.Project, req.Realm)
	} else {
		realm = realms.Join(req.Project, realms.LegacyRealm)
	}

	// Confirm that the Prefix is a valid stream name.
	prefix := types.StreamName(req.Prefix)
	if err := prefix.Validate(); err != nil {
		log.WithError(err).Warningf(c, "Invalid prefix.")
		return nil, status.Errorf(codes.InvalidArgument, "invalid prefix")
	}

	// Check the caller is allowed to register prefixes in the requested realm.
	if err := coordinator.CheckPermission(c, coordinator.PermLogsCreate, prefix, realm); err != nil {
		return nil, err
	}

	pubsubTopic, expiration, err := getTopicAndExpiration(c, req)
	if err != nil {
		log.WithError(err).Errorf(c, "Cannot get pubsubTopic")
		return nil, status.Errorf(codes.Internal, "unable to get pubsubTopic")
	}

	// Determine our prefix expiration. This must be > 0, else there will be no
	// window when log streams can be registered and this prefix is useless.
	//
	// We will choose the shortest expiration window defined by our request and
	// our project and service configurations.
	if expiration <= 0 {
		log.Errorf(c, "Refusing to register prefix in expired state.")
		return nil, status.Errorf(codes.InvalidArgument, "no prefix expiration defined")
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
		return nil, status.Error(codes.AlreadyExists, "the prefix is already registered")

	default:
		log.WithError(err).Errorf(c, "Failed to check for existing prefix (non-transactional).")
		return nil, status.Error(codes.Internal, "internal server error")
	}

	// The prefix doesn't appear to be registered. Prepare to transactionally
	// register it.
	secret := make(types.PrefixSecret, types.PrefixSecretLength)
	if _, err := cryptorand.Read(c, []byte(secret)); err != nil {
		log.WithError(err).Errorf(c, "Failed to generate prefix secret.")
		return nil, status.Error(codes.Internal, "internal server error")
	}

	pfx.Created = clock.Now(c).UTC()
	pfx.Prefix = string(prefix)
	pfx.Realm = realm
	pfx.Source = req.SourceInfo
	pfx.Secret = []byte(secret)
	pfx.Expiration = pfx.Created.Add(expiration)
	pfx.OpNonce = req.OpNonce
	if err := pfx.Validate(); err != nil {
		log.WithError(err).Errorf(c, "Invalid LogPrefix.")
		return nil, status.Errorf(codes.InvalidArgument, "LogPrefix definition invalid")
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
			return status.Error(codes.AlreadyExists, "the prefix is already registered")

		default:
			log.WithError(err).Errorf(c, "Failed to check for existing prefix (transactional).")
			return status.Error(codes.Internal, "internal server error")
		}

		if err := ds.Put(c, pfx); err != nil {
			log.WithError(err).Errorf(c, "Failed to register prefix.")
			return status.Error(codes.Internal, "internal server error")
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
