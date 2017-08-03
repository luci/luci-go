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

package coordinator

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/luci/luci-go/appengine/gaeauth/server/gaesigner"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/gcloud/gs"
	"github.com/luci/luci-go/common/gcloud/pubsub"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/logdog/api/config/svcconfig"
	"github.com/luci/luci-go/logdog/appengine/coordinator/config"
	"github.com/luci/luci-go/logdog/common/storage"
	"github.com/luci/luci-go/logdog/common/storage/archive"
	"github.com/luci/luci-go/logdog/common/storage/bigtable"
	"github.com/luci/luci-go/logdog/common/storage/caching"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/router"

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
	Config(context.Context) (*config.Config, error)

	// ProjectConfig returns the project configuration for the named project.
	//
	// The production instance will cache the results for the duration of the
	// request.
	//
	// Returns the same error codes as config.ProjectConfig.
	ProjectConfig(context.Context, cfgtypes.ProjectName) (*svcconfig.ProjectConfig, error)

	// Storage returns a Storage instance for the supplied log stream.
	//
	// The caller must close the returned instance if successful.
	StorageForStream(context.Context, *LogStreamState) (Storage, error)

	// ArchivalPublisher returns an ArchivalPublisher instance.
	ArchivalPublisher(context.Context) (ArchivalPublisher, error)
}

// ProdCoordinatorService is Middleware used by Coordinator services.
//
// It installs a production Services instance into the Context.
func ProdCoordinatorService(c *router.Context, next router.Handler) {
	services := prodServicesInst{
		aeCtx: appengine.WithContext(c.Context, c.Request),
	}
	defer services.close(c.Context)

	c.Context = WithServices(c.Context, &services)
	next(c)
}

// prodServicesInst is a Service exposing production faciliites. A unique
// instance is bound to each each request.
type prodServicesInst struct {
	sync.Mutex

	// aeCtx is an AppEngine Context initialized for the current request.
	aeCtx context.Context

	// gcfg is the cached global configuration.
	gcfg           *config.Config
	projectConfigs map[cfgtypes.ProjectName]*cachedProjectConfig

	// archivalIndex is the atomically-manipulated archival index for the
	// ArchivalPublisher. This is shared between all ArchivalPublisher instances
	// from this service.
	archivalIndex int32

	// pubSubClient is a Pub/Sub client generated during this request.
	//
	// It will be closed on "close".
	pubSubClient *vkit.PublisherClient
	// signer is the signer instance to use.
	signer gaesigner.Signer
}

func (s *prodServicesInst) close(c context.Context) {
	s.Lock()
	defer s.Unlock()

	if client := s.pubSubClient; client != nil {
		if err := client.Close(); err != nil {
			log.WithError(err).Errorf(c, "Failed to close Pub/Sub client singleton.")
		}
		s.pubSubClient = nil
	}
}
func (s *prodServicesInst) Config(c context.Context) (*config.Config, error) {
	s.Lock()
	defer s.Unlock()

	// Load/cache the global config.
	if s.gcfg == nil {
		var err error
		s.gcfg, err = config.Load(c)
		if err != nil {
			return nil, err
		}
	}

	return s.gcfg, nil
}

// cachedProjectConfig is a singleton instance that holds a project config
// state. It is populated when resolve is called, and is goroutine-safe for
// read-only operations.
type cachedProjectConfig struct {
	sync.Once

	project cfgtypes.ProjectName
	pcfg    *svcconfig.ProjectConfig
	err     error
}

func (cp *cachedProjectConfig) resolve(c context.Context) (*svcconfig.ProjectConfig, error) {
	// Load the project config exactly once. This will be cached for the remainder
	// of this request.
	//
	// If multiple goroutines attempt to load it, exactly one will, and the rest
	// will block. All operations after this Once must be read-only.
	cp.Do(func() {
		cp.pcfg, cp.err = config.ProjectConfig(c, cp.project)
	})
	return cp.pcfg, cp.err
}

func (s *prodServicesInst) getOrCreateCachedProjectConfig(project cfgtypes.ProjectName) *cachedProjectConfig {
	s.Lock()
	defer s.Unlock()

	if s.projectConfigs == nil {
		s.projectConfigs = make(map[cfgtypes.ProjectName]*cachedProjectConfig)
	}
	cp := s.projectConfigs[project]
	if cp == nil {
		cp = &cachedProjectConfig{
			project: project,
		}
		s.projectConfigs[project] = cp
	}
	return cp
}

func (s *prodServicesInst) ProjectConfig(c context.Context, project cfgtypes.ProjectName) (*svcconfig.ProjectConfig, error) {
	return s.getOrCreateCachedProjectConfig(project).resolve(c)
}

func (s *prodServicesInst) StorageForStream(c context.Context, lst *LogStreamState) (Storage, error) {
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

func (s *prodServicesInst) newBigTableStorage(c context.Context) (Storage, error) {
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

	// Get an Authenticator bound to the token scopes that we need for BigTable.
	creds, err := auth.GetPerRPCCredentials(auth.AsSelf, auth.WithScopes(bigtable.StorageScopes...))
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create BigTable credentials.")
		return nil, errors.New("failed to create BigTable credentials")
	}

	// Explicitly clear gRPC metadata from the Context. It is forwarded to
	// delegate calls by default, and standard request metadata can break BigTable
	// calls.
	c = metadata.NewContext(c, nil)

	st, err := bigtable.New(c, bigtable.Options{
		Project:  bt.Project,
		Instance: bt.Instance,
		LogTable: bt.LogTableName,
		ClientOptions: []option.ClientOption{
			option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)),
		},
		Cache: s.getStorageCache(),
	})
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create BigTable instance.")
		return nil, err
	}

	return &bigTableStorage{
		Storage: st,
	}, nil
}

func (s *prodServicesInst) newGoogleStorage(c context.Context, index, stream gs.Path) (Storage, error) {
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

	st, err := archive.New(c, archive.Options{
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

func (s *prodServicesInst) ArchivalPublisher(c context.Context) (ArchivalPublisher, error) {
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
	return &pubsubArchivalPublisher{
		publisher: &pubsub.UnbufferedPublisher{
			Topic:  fullTopic,
			Client: psClient,
		},
		publishIndexFunc: s.nextArchiveIndex,
	}, nil
}

func (s *prodServicesInst) getPubSubClient() (*vkit.PublisherClient, error) {
	s.Lock()
	defer s.Unlock()

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

var storageCacheSingleton StorageCache

func (s *prodServicesInst) getStorageCache() caching.Cache { return &storageCacheSingleton }

// Storage is an interface to storage used by the Coordinator.
type Storage interface {
	// Storage is the base Storage instance.
	storage.Storage

	// GetSignedURLs attempts to sign the storage's stream's RecordIO archive
	// stream storage URL.
	//
	// If signing is not supported by this Storage instance, this will return
	// a nil signing response and no error.
	GetSignedURLs(context.Context, *URLSigningRequest) (*URLSigningResponse, error)
}

// URLSigningRequest is the set of URL signing parameters passed to a
// Storage.GetSignedURLs call.
type URLSigningRequest struct {
	// Expriation is the signed URL expiration time.
	Lifetime time.Duration

	// Stream, if true, requests a signed log stream URL.
	Stream bool
	// Index, if true, requests a signed log stream index URL.
	Index bool
}

// HasWork returns true if this signing request actually has work that is
// requested.
func (r *URLSigningRequest) HasWork() bool {
	return (r.Stream || r.Index) && (r.Lifetime > 0)
}

// URLSigningResponse is the resulting signed URLs from a Storage.GetSignedURLs
// call.
type URLSigningResponse struct {
	// Expriation is the signed URL expiration time.
	Expiration time.Time

	// Stream is the signed URL for the log stream, if requested.
	Stream string
	// Index is the signed URL for the log stream index, if requested.
	Index string
}

// intermediateStorage is a Storage instance bound to BigTable.
type bigTableStorage struct {
	// Storage is the base storage.Storage instance.
	storage.Storage
}

func (*bigTableStorage) GetSignedURLs(context.Context, *URLSigningRequest) (*URLSigningResponse, error) {
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

func (si *googleStorage) GetSignedURLs(c context.Context, req *URLSigningRequest) (*URLSigningResponse, error) {
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
	resp := URLSigningResponse{
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
