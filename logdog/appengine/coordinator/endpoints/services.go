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

// ProdService is an instance-global configuration for production
// Coordinator services. A zero-value struct should be used.
//
// It can be installed via middleware using its Base method.
type ProdService struct {
	pubSubLock   sync.Mutex
	pubSubClient *vkit.PublisherClient
}

func (svc *ProdService) getPubSubClient(c context.Context) (*vkit.PublisherClient, error) {
	svc.pubSubLock.Lock()
	defer svc.pubSubLock.Unlock()

	if svc.pubSubClient != nil {
		return svc.pubSubClient, nil
	}

	// Create a new AppEngine context. Don't pass gRPC metadata to PubSub, since
	// we don't want any caller RPC to be forwarded to the backend service.
	c = metadata.NewOutgoingContext(c, nil)

	// Create an authenticated global Pub/Sub client.
	creds, err := auth.GetPerRPCCredentials(auth.AsSelf, auth.WithScopes(pubsub.PublisherScopes...))
	if err != nil {
		return nil, errors.Annotate(err, "failed to create Pub/Sub credentials").Err()
	}

	client, err := vkit.NewPublisherClient(c,
		option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)))
	if err != nil {
		return nil, errors.Annotate(err, "Failed to create Pub/Sub client").Err()
	}

	svc.pubSubClient = client
	return client, nil
}

// Base is Middleware used by Coordinator services.
//
// It installs a production Services instance into the Context.
func (svc *ProdService) Base(c *router.Context, next router.Handler) {
	services := prodServicesInst{
		ProdService: svc,
		aeCtx:       appengine.WithContext(c.Context, c.Request),
	}

	c.Context = coordinator.WithConfigProvider(c.Context, &services)
	c.Context = WithServices(c.Context, &services)
	next(c)
}

// prodServicesInst is a Service exposing production faciliites. A unique
// instance is bound to each each request.
type prodServicesInst struct {
	*ProdService
	sync.Mutex

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

	// Create a new Pub/Sub client. Use our AppEngine Context, since we need to
	// execute a socket API call.
	client, err := s.getPubSubClient(s.aeCtx)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Pub/Sub client.")
		return nil, err
	}

	// Create a Topic, and configure it to not bundle messages.
	return &coordinator.PubsubArchivalPublisher{
		Publisher: &pubsub.UnbufferedPublisher{
			Topic:  fullTopic,
			Client: client,
		},
		AECtx:            s.aeCtx,
		PublishIndexFunc: s.nextArchiveIndex,
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
