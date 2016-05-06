// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/julienschmidt/httprouter"
	gaeauthClient "github.com/luci/luci-go/appengine/gaeauth/client"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/gcloud/gs"
	"github.com/luci/luci-go/common/gcloud/pubsub"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"github.com/luci/luci-go/server/logdog/storage"
	"github.com/luci/luci-go/server/logdog/storage/bigtable"
	"github.com/luci/luci-go/server/middleware"
	"golang.org/x/net/context"
	"google.golang.org/cloud"
	gcps "google.golang.org/cloud/pubsub"
	"google.golang.org/grpc/metadata"
)

// Services is a set of support services used by Coordinator.
//
// Each Services instance is valid for a singel request, but can be re-used
// throughout that request. This is advised, as the Services instance may
// optionally cache values.
//
// Services methods are goroutine-safe.
//
// By default, a production set of services will be used. However, this can be
// overridden for testing to mock the service layer.
type Services interface {
	// Config returns the current instance and application configuration
	// instances.
	//
	// The production instance will cache the results for the duration of the
	// request.
	Config(context.Context) (*config.GlobalConfig, *svcconfig.Config, error)

	// Storage returns an intermediate storage instance for use by this service.
	//
	// The caller must close the returned instance if successful.
	IntermediateStorage(context.Context) (storage.Storage, error)

	// GSClient instantiates a Google Storage client.
	GSClient(context.Context) (gs.Client, error)

	// ArchivalPublisher returns an ArchivalPublisher instance.
	ArchivalPublisher(context.Context) (ArchivalPublisher, error)
}

// WithProdServices is a middleware Handler that installs a production Services
// instance into its Context.
func WithProdServices(h middleware.Handler) middleware.Handler {
	return func(c context.Context, rw http.ResponseWriter, r *http.Request, params httprouter.Params) {
		c = UseProdServices(c)
		h(c, rw, r, params)
	}
}

// UseProdServices installs production Services instance into the supplied
// Context.
func UseProdServices(c context.Context) context.Context {
	return WithServices(c, &prodServicesInst{})
}

// prodServicesInst is a Service exposing production faciliites. A unique
// instance is bound to each each request.
type prodServicesInst struct {
	sync.Mutex

	// gcfg is the cached global configuration.
	gcfg *config.GlobalConfig
	// cfg is the cached configuration.
	cfg *svcconfig.Config

	// archivalIndex is the atomically-manipulated archival index for the
	// ArchivalPublisher. This is shared between all ArchivalPublisher instances
	// from this service.
	archivalIndex int32
}

// Config returns the current instance and application configuration instances.
//
// After a success, successive calls will return a cached result.
func (s *prodServicesInst) Config(c context.Context) (*config.GlobalConfig, *svcconfig.Config, error) {
	s.Lock()
	defer s.Unlock()

	// Load/cache the global config.
	if s.gcfg == nil {
		var err error
		s.gcfg, err = config.LoadGlobalConfig(c)
		if err != nil {
			return nil, nil, err
		}
	}

	if s.cfg == nil {
		var err error
		s.cfg, err = s.gcfg.LoadConfig(c)
		if err != nil {
			return nil, nil, err
		}
	}

	return s.gcfg, s.cfg, nil
}

func (s *prodServicesInst) IntermediateStorage(c context.Context) (storage.Storage, error) {
	gcfg, cfg, err := s.Config(c)
	if err != nil {
		return nil, err
	}

	// Is BigTable configured?
	if cfg.Storage == nil {
		return nil, errors.New("no storage configuration")
	}

	bt := cfg.Storage.GetBigtable()
	if bt == nil {
		return nil, errors.New("no BigTable configuration")
	}

	// Validate the BigTable configuration.
	log.Fields{
		"project":      bt.Project,
		"zone":         bt.Zone,
		"cluster":      bt.Cluster,
		"logTableName": bt.LogTableName,
	}.Debugf(c, "Connecting to BigTable.")
	var merr errors.MultiError
	if bt.Project == "" {
		merr = append(merr, errors.New("missing project"))
	}
	if bt.Zone == "" {
		merr = append(merr, errors.New("missing zone"))
	}
	if bt.Cluster == "" {
		merr = append(merr, errors.New("missing cluster"))
	}
	if bt.LogTableName == "" {
		merr = append(merr, errors.New("missing log table name"))
	}
	if len(merr) > 0 {
		return nil, merr
	}

	// Get an Authenticator bound to the token scopes that we need for BigTable.
	a, err := gaeauthClient.Authenticator(c, bigtable.StorageScopes, gcfg.BigTableServiceAccountJSON)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create BigTable authenticator.")
		return nil, errors.New("failed to create BigTable authenticator")
	}

	// Explicitly clear gRPC metadata from the Context. It is forwarded to
	// delegate calls by default, and standard request metadata can break BigTable
	// calls.
	c = metadata.NewContext(c, nil)

	st, err := bigtable.New(c, bigtable.Options{
		Project:  bt.Project,
		Zone:     bt.Zone,
		Cluster:  bt.Cluster,
		LogTable: bt.LogTableName,
		ClientOptions: []cloud.ClientOption{
			cloud.WithTokenSource(a.TokenSource()),
		},
	})
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create BigTable instance.")
		return nil, err
	}
	return st, nil
}

func (s *prodServicesInst) GSClient(c context.Context) (gs.Client, error) {
	// Get an Authenticator bound to the token scopes that we need for
	// authenticated Cloud Storage access.
	rt, err := gaeauthClient.Transport(c, gs.ReadOnlyScopes, nil)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Cloud Storage transport.")
		return nil, errors.New("failed to create Cloud Storage transport")
	}
	return gs.NewProdClient(c, rt)
}

func (s *prodServicesInst) ArchivalPublisher(c context.Context) (ArchivalPublisher, error) {
	_, cfg, err := s.Config(c)
	if err != nil {
		return nil, err
	}

	fullTopic := pubsub.Topic(cfg.Coordinator.ArchiveTopic)
	if err := fullTopic.Validate(); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"topic":      fullTopic,
		}.Errorf(c, "Failed to validate archival topic.")
		return nil, errors.New("invalid archival topic")
	}
	project, topic := fullTopic.Split()

	// Create an authenticated Pub/Sub client.
	// Pub/Sub topic publishing.
	auth, err := gaeauthClient.Authenticator(c, pubsub.PublisherScopes, nil)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Pub/Sub authenticator.")
		return nil, errors.New("failed to create Pub/Sub authenticator")
	}

	client, err := auth.Client()
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Pub/Sub HTTP client.")
		return nil, errors.New("failed to create Pub/Sub HTTP client")
	}

	psClient, err := gcps.NewClient(c, project, cloud.WithBaseHTTP(client))
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Pub/Sub client.")
		return nil, errors.New("failed to create Pub/Sub client")
	}

	return &pubsubArchivalPublisher{
		topic:            psClient.Topic(topic),
		publishIndexFunc: s.nextArchiveIndex,
	}, nil
}

func (s *prodServicesInst) nextArchiveIndex() uint64 {
	// We use a 32-bit value for this because it avoids atomic memory bounary
	// issues. Furthermore, we constrain it to be positive, using a negative
	// value as a sentinel that the archival index has wrapped.
	//
	// This is reasonable, as it is very unlikely that a single request will issue
	// more than MaxInt32 archival tasks.
	v := atomic.AddInt32(&s.archivalIndex, 1) - 1
	if v < 0 {
		panic("archival index has wrapped")
	}
	return uint64(v)
}
