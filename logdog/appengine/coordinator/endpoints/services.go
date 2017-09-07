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
	"time"

	"go.chromium.org/luci/appengine/gaeauth/server/gaesigner"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/gcloud/pubsub"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/storage/archive"
	"go.chromium.org/luci/logdog/common/storage/bigtable"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	gcbt "cloud.google.com/go/bigtable"
	vkit "cloud.google.com/go/pubsub/apiv1"
	gcst "cloud.google.com/go/storage"
	"google.golang.org/api/option"
	"google.golang.org/appengine"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"golang.org/x/net/context"
)

const (
	// maxSignedURLLifetime is the maximum allowed signed URL lifetime.
	maxSignedURLLifetime = 1 * time.Hour

	// maxGSFetchSize is the maximum amount of data we can fetch from a single
	// Google Storage RPC call.
	//
	// AppEngine's "urlfetch" has a limit of 32MB:
	// https://cloud.google.com/appengine/docs/python/outbound-requests
	//
	// We're choosing a smaller window. It may cause extra urlfetch runs, but
	// it also reduces the maximum amount of memory that we need, since urlfetch
	// loads in chunks.
	maxGSFetchSize = int64(8 * 1024 * 1024)
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

	// Storage returns a Storage instance for the supplied log stream.
	//
	// The caller must close the returned instance if successful.
	StorageForStream(context.Context, *coordinator.LogStreamState) (coordinator.SigningStorage, error)

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

	bigTableLock     sync.Mutex
	bigTableClient   *gcbt.Client
	bigTableProject  string
	bigTableInstance string
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

func (svc *ProdService) getBigTableClient(c context.Context, project, instance string) (*gcbt.Client, error) {
	svc.bigTableLock.Lock()
	defer svc.bigTableLock.Unlock()

	// Reuse the existing client if parameters haven't changed.
	if svc.bigTableClient != nil {
		if svc.bigTableProject == project && svc.bigTableInstance == instance {
			return svc.bigTableClient, nil
		}
		if err := svc.bigTableClient.Close(); err != nil {
			log.WithError(err).Warningf(c, "Failed to close current BigTable client.")
		}
		svc.bigTableClient = nil
	}

	// Create a new AppEngine context. Don't pass gRPC metadata to PubSub, since
	// we don't want any caller RPC to be forwarded to the backend service.
	c = metadata.NewOutgoingContext(c, nil)

	// Get an Authenticator bound to the token scopes that we need for BigTable.
	creds, err := auth.GetPerRPCCredentials(auth.AsSelf, auth.WithScopes(bigtable.StorageScopes...))
	if err != nil {
		return nil, errors.Annotate(err, "failed to create BigTable credentials").Err()
	}

	// We either don't have a BigTable client, or we need to regenerate a new
	// BigTable client with different parameters.
	opts := bigtable.DefaultClientOptions()
	opts = append(opts, option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)))
	client, err := gcbt.NewClient(c, project, instance, opts...)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create BigTable client").Err()
	}

	svc.bigTableClient = client
	svc.bigTableProject = project
	svc.bigTableInstance = instance
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

func (s *prodServicesInst) StorageForStream(c context.Context, lst *coordinator.LogStreamState) (
	coordinator.SigningStorage, error) {

	if !lst.ArchivalState().Archived() {
		log.Debugf(c, "Log is not archived. Fetching from intermediate storage.")
		return s.newBigTableStorage(c)
	}

	log.Fields{
		"indexURL":    lst.ArchiveIndexURL,
		"streamURL":   lst.ArchiveStreamURL,
		"archiveTime": lst.ArchivedTime,
	}.Debugf(c, "Log is archived. Fetching from archive storage.")
	return s.newGoogleStorage(c, gs.Path(lst.ArchiveIndexURL), gs.Path(lst.ArchiveStreamURL))
}

func (s *prodServicesInst) newBigTableStorage(c context.Context) (coordinator.SigningStorage, error) {
	cfg, err := s.Config(c)
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
		"instance":     bt.Instance,
		"logTableName": bt.LogTableName,
	}.Debugf(c, "Connecting to BigTable.")
	var merr errors.MultiError
	if bt.Project == "" {
		merr = append(merr, errors.New("missing project"))
	}
	if bt.Instance == "" {
		merr = append(merr, errors.New("missing instance"))
	}
	if bt.LogTableName == "" {
		merr = append(merr, errors.New("missing log table name"))
	}
	if len(merr) > 0 {
		return nil, merr
	}

	client, err := s.getBigTableClient(c, bt.Project, bt.Instance)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create a new BigTable client.")
		return nil, err
	}

	return &bigTableStorage{
		Storage: &bigtable.Storage{
			Client:   client,
			Cache:    s.getStorageCache(),
			LogTable: bt.LogTableName,
		},
		client: client,
	}, nil
}

func (s *prodServicesInst) newGoogleStorage(c context.Context, index, stream gs.Path) (coordinator.SigningStorage, error) {
	gs, err := s.newGSClient(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Google Storage client.")
		return nil, err
	}
	defer func() {
		if gs != nil {
			if err := gs.Close(); err != nil {
				log.WithError(err).Warningf(c, "Failed to close Google Storage client.")
			}
		}
	}()

	st, err := archive.New(archive.Options{
		Index:  index,
		Stream: stream,
		Client: gs,
		Cache:  s.getStorageCache(),
	})
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Google Storage storage instance.")
		return nil, err
	}

	rv := &googleStorage{
		Storage: st,
		svc:     s,
		gs:      gs,
		stream:  stream,
		index:   index,
	}
	gs = nil // Don't close in defer.
	return rv, nil
}

func (s *prodServicesInst) newGSClient(c context.Context) (gs.Client, error) {
	// Get an Authenticator bound to the token scopes that we need for
	// authenticated Cloud Storage access.
	transport, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(gs.ReadOnlyScopes...))
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Cloud Storage transport.")
		return nil, errors.New("failed to create Cloud Storage transport")
	}
	prodClient, err := gs.NewProdClient(c, transport)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create GS client.")
		return nil, err
	}

	// Wrap the GS client in a limiter. This prevents large requests from
	// exceeding the urlfetch threshold.
	return &gs.LimitedClient{
		Client:       prodClient,
		MaxReadBytes: maxGSFetchSize,
	}, nil
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

var storageCacheSingleton coordinator.StorageCache

func (s *prodServicesInst) getStorageCache() storage.Cache { return &storageCacheSingleton }

// bigTableStorage is a Storage instance bound to BigTable.
type bigTableStorage struct {
	// Storage is the base storage.Storage instance.
	storage.Storage

	client *gcbt.Client
}

func (st *bigTableStorage) Close() {
	st.Storage.Close()
}

func (*bigTableStorage) GetSignedURLs(context.Context, *coordinator.URLSigningRequest) (
	*coordinator.URLSigningResponse, error) {

	return nil, nil
}

type googleStorage struct {
	// Storage is the base storage.Storage instance.
	storage.Storage
	// svc is the services instance that created this.
	svc *prodServicesInst

	// ctx is the Context that was bound at the time of of creation.
	ctx context.Context
	// gs is the backing Google Storage client.
	gs gs.Client

	// stream is the stream's Google Storage URL.
	stream gs.Path
	// index is the index's Google Storage URL.
	index gs.Path

	gsSigningOpts func(context.Context) (*gcst.SignedURLOptions, error)
}

func (si *googleStorage) Close() {
	if err := si.gs.Close(); err != nil {
		log.WithError(err).Warningf(si.ctx, "Failed to close Google Storage client.")
	}
	si.Storage.Close()
}

func (si *googleStorage) GetSignedURLs(c context.Context, req *coordinator.URLSigningRequest) (
	*coordinator.URLSigningResponse, error) {

	info, err := si.svc.signer.ServiceInfo(c)
	if err != nil {
		return nil, errors.Annotate(err, "").InternalReason("failed to get service info").Err()
	}

	lifetime := req.Lifetime
	switch {
	case lifetime < 0:
		return nil, errors.Reason("invalid signed URL lifetime: %s", lifetime).Err()

	case lifetime > maxSignedURLLifetime:
		lifetime = maxSignedURLLifetime
	}

	// Get our signing options.
	resp := coordinator.URLSigningResponse{
		Expiration: clock.Now(c).Add(lifetime),
	}
	opts := gcst.SignedURLOptions{
		GoogleAccessID: info.ServiceAccountName,
		SignBytes: func(b []byte) ([]byte, error) {
			_, signedBytes, err := si.svc.signer.SignBytes(c, b)
			return signedBytes, err
		},
		Method:  "GET",
		Expires: resp.Expiration,
	}

	doSign := func(path gs.Path) (string, error) {
		url, err := gcst.SignedURL(path.Bucket(), path.Filename(), &opts)
		if err != nil {
			return "", errors.Annotate(err, "").InternalReason(
				"failed to sign URL: bucket(%s)/filename(%s)", path.Bucket(), path.Filename).Err()
		}
		return url, nil
	}

	// Sign stream URL.
	if req.Stream {
		if resp.Stream, err = doSign(si.stream); err != nil {
			return nil, errors.Annotate(err, "").InternalReason("failed to sign stream URL").Err()
		}
	}

	// Sign index URL.
	if req.Index {
		if resp.Index, err = doSign(si.index); err != nil {
			return nil, errors.Annotate(err, "").InternalReason("failed to sign index URL").Err()
		}
	}

	return &resp, nil
}
