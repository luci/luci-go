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

package endpoints

import (
	"sync"
	"sync/atomic"

	"go.chromium.org/luci/appengine/gaeauth/server/gaesigner"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/pubsub"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	vkit "cloud.google.com/go/pubsub/apiv1"
	"google.golang.org/api/option"
	"google.golang.org/appengine"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"golang.org/x/net/context"
)

// Services is a set of support services used by AppEngine Classic Coordinator
// endpoints.
//
// Each instance is valid for a single request, but can be re-used throughout
// that request. This is advised, as the Services instance may optionally cache
// values.
//
// Services methods are goroutine-safe.
type Services interface {
	coordinator.ConfigProvider

	// ArchivalPublisher returns an ArchivalPublisher instance.
	ArchivalPublisher(context.Context) (coordinator.ArchivalPublisher, error)
}

// ProdServices is Middleware used by Coordinator services.
//
// It installs a production Services instance into the Context.
func ProdServices(c *router.Context, next router.Handler) {
	services := prodServicesInst{
		aeCtx: appengine.WithContext(c.Context, c.Request),
	}
	defer services.close(c.Context)

	c.Context = coordinator.WithConfigProvider(c.Context, &services)
	c.Context = WithServices(c.Context, &services)
	next(c)
}

// prodServicesInst is a Service exposing production faciliites. A unique
// instance is bound to each each request.
type prodServicesInst struct {
	// LUCIConfigProvider satisfies the ConfigProvider interface requirement.
	coordinator.LUCIConfigProvider

	// aeCtx is an AppEngine Context initialized for the current request.
	aeCtx context.Context

	// archivalIndex is the atomically-manipulated archival index for the
	// ArchivalPublisher. This is shared between all ArchivalPublisher instances
	// from this service.
	archivalIndex int32

	// pubSubLock protects pubSubClient member.
	pubSubLock sync.Mutex
	// pubSubClient is a Pub/Sub client generated during this request.
	//
	// It will be closed on "close".
	pubSubClient *vkit.PublisherClient
	// signer is the signer instance to use.
	signer gaesigner.Signer
}

func (s *prodServicesInst) close(c context.Context) {
	s.pubSubLock.Lock()
	defer s.pubSubLock.Unlock()

	if client := s.pubSubClient; client != nil {
		if err := client.Close(); err != nil {
			log.WithError(err).Errorf(c, "Failed to close Pub/Sub client singleton.")
		}
		s.pubSubClient = nil
	}
}

func (s *prodServicesInst) ArchivalPublisher(c context.Context) (coordinator.ArchivalPublisher, error) {
	cfg, err := s.Config(c)
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

	psClient, err := s.getPubSubClient()
	if err != nil {
		return nil, err
	}

	// Create a Topic, and configure it to not bundle messages.
	return &coordinator.PubsubArchivalPublisher{
		Publisher: &pubsub.UnbufferedPublisher{
			Topic:  fullTopic,
			Client: psClient,
		},
		PublishIndexFunc: s.nextArchiveIndex,
	}, nil
}

func (s *prodServicesInst) getPubSubClient() (*vkit.PublisherClient, error) {
	s.pubSubLock.Lock()
	defer s.pubSubLock.Unlock()

	if s.pubSubClient != nil {
		return s.pubSubClient, nil
	}

	// Create a new AppEngine context. Don't pass gRPC metadata to PubSub, since
	// we don't want any caller RPC to be forwarded to the backend service.
	c := metadata.NewContext(s.aeCtx, nil)

	// Create an authenticated unbuffered Pub/Sub Publisher.
	creds, err := auth.GetPerRPCCredentials(auth.AsSelf, auth.WithScopes(pubsub.PublisherScopes...))
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Pub/Sub credentials.")
		return nil, errors.New("failed to create Pub/Sub credentials")
	}

	psClient, err := vkit.NewPublisherClient(c,
		option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)))
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Pub/Sub client.")
		return nil, errors.New("failed to create Pub/Sub client")
	}

	s.pubSubClient = psClient
	return psClient, nil
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
