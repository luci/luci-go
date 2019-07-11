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

package flex

import (
	"context"
	"time"

	"go.chromium.org/luci/appengine/gaeauth/server/gaesigner"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	log "go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/config"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/storage/archive"
	"go.chromium.org/luci/logdog/common/storage/bigtable"
	"go.chromium.org/luci/logdog/common/types"

	gcbt "cloud.google.com/go/bigtable"
	gcst "cloud.google.com/go/storage"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

const (
	// maxSignedURLLifetime is the maximum allowed signed URL lifetime.
	maxSignedURLLifetime = 1 * time.Hour
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
	StorageForStream(context.Context, *coordinator.LogStreamState, types.ProjectName) (coordinator.SigningStorage, error)
}

// GlobalServices is an application singleton that stores cross-request service
// structures.
//
// It is applied to each Flex HTTP request using its Base() middleware method.
type GlobalServices struct {
	// LUCIConfigProvider satisfies the ConfigProvider interface requirement.
	coordinator.LUCIConfigProvider

	// Signer is the signer instance to use.
	Signer gaesigner.Signer

	// gsClient is the application-global Google Storage client.
	btStorage *bigtable.Storage

	// gsClientFactory is the application-global creator of Google Storage clients.
	gsClientFactory func(context.Context, types.ProjectName) (gs.Client, error)

	// storageCache is the process-wide cache used for storing Storage data.
	storageCache *StorageCache
}

// NewGlobalServices instantiates a new GlobalServices instance.
//
// The Context passed to GlobalServices should be a global Context not a
// request-specific Context, with required services installed:
// - auth
// - luci_config
func NewGlobalServices(c context.Context) (*GlobalServices, error) {
	var err error

	// Instantiate our services. At the moment, it doesn't have instantiated
	// clients, so it's only partially viable. We will use it to fetch our
	// application configuration, which we will in turn use to instantiate our
	// clients.
	s := GlobalServices{
		storageCache: &StorageCache{},
	}

	// Load our service configuration.
	cfg, err := s.Config(c)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get service configuration").Err()
	}

	// Connect our clients.
	if err := s.connectBigTableClient(c, cfg); err != nil {
		return nil, errors.Annotate(err, "failed to connect BigTable client").Err()
	}

	if err := s.createGoogleStorageClientFactory(c, cfg); err != nil {
		return nil, errors.Annotate(err, "failed to connect Google Storage client").Err()
	}

	return &s, nil
}

func (gsvc *GlobalServices) connectBigTableClient(c context.Context, cfg *config.Config) error {
	// Is BigTable configured?
	if cfg.Storage == nil {
		return errors.New("no storage configuration")
	}
	bt := cfg.Storage.GetBigtable()
	if bt == nil {
		return errors.New("no BigTable configuration")
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
		return merr
	}

	// Get an Authenticator bound to the token scopes that we need for BigTable.
	creds, err := auth.GetPerRPCCredentials(auth.AsSelf, auth.WithScopes(bigtable.StorageScopes...))
	if err != nil {
		return errors.Annotate(err, "failed to create BigTable credentials").Err()
	}

	opts := bigtable.DefaultClientOptions()
	opts = append(opts, option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)))
	client, err := gcbt.NewClient(c, bt.Project, bt.Instance, opts...)
	if err != nil {
		return errors.Annotate(err, "failed to create BigTable client").Err()
	}

	gsvc.btStorage = &bigtable.Storage{
		Client:   client,
		LogTable: bt.LogTableName,
		Cache:    gsvc.storageCache,
	}
	return nil
}

func (gsvc *GlobalServices) createGoogleStorageClientFactory(c context.Context, cfg *config.Config) error {
	gsvc.gsClientFactory = func(c context.Context, project types.ProjectName) (client gs.Client, e error) {
		// TODO(vadimsh): Switch to AsProject + WithProject(project.String()) once
		// we are ready to roll out project scoped service accounts in Logdog.
		transport, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(gs.ReadOnlyScopes...))
		if err != nil {
			return nil, errors.Annotate(err, "failed to create Google Storage RPC transport").Err()
		}
		prodClient, err := gs.NewProdClient(c, transport)
		if err != nil {
			return nil, errors.Annotate(err, "Failed to create GS client.").Err()
		}
		return prodClient, nil
	}
	return nil
}

// Base is Middleware used by Coordinator Flex services.
//
// It installs a production Services instance into the Context.
func (gsvc *GlobalServices) Base(c *router.Context, next router.Handler) {
	services := flexServicesInst{
		GlobalServices: gsvc,
	}

	c.Context = coordinator.WithConfigProvider(c.Context, &services)
	c.Context = WithServices(c.Context, &services)
	next(c)
}

// Close closes the GlobalServices instance, releasing any retained resources.
func (gsvc *GlobalServices) Close() error {
	return nil
}

// flexServicesInst is a Service exposing production faciliites for the
// AppEngine Flex environment. A unique instance is bound to each each request.
type flexServicesInst struct {
	// GlobalServices is the base services singleton.
	*GlobalServices
}

func (s *flexServicesInst) StorageForStream(c context.Context, lst *coordinator.LogStreamState, project types.ProjectName) (
	coordinator.SigningStorage, error) {

	if !lst.ArchivalState().Archived() {
		log.Debugf(c, "Log is not archived. Fetching from intermediate storage.")
		return noSignedURLStorage{s.btStorage}, nil
	}

	gsClient, err := s.gsClientFactory(c, project)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Google Storage client.")
		return nil, err
	}

	log.Fields{
		"indexURL":    lst.ArchiveIndexURL,
		"streamURL":   lst.ArchiveStreamURL,
		"archiveTime": lst.ArchivedTime,
	}.Debugf(c, "Log is archived. Fetching from archive storage.")

	st, err := archive.New(archive.Options{
		Index:  gs.Path(lst.ArchiveIndexURL),
		Stream: gs.Path(lst.ArchiveStreamURL),
		Cache:  s.storageCache,
		Client: gsClient,
	})
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Google Storage storage instance.")
		return nil, err
	}

	rv := &googleStorage{
		Storage: st,
		svc:     s,
		gs:      gsClient,
		stream:  gs.Path(lst.ArchiveStreamURL),
		index:   gs.Path(lst.ArchiveIndexURL),
	}
	return rv, nil
}

// noSignedURLStorage is a thin wrapper around a Storage instance that cannot
// sign URLs.
type noSignedURLStorage struct {
	storage.Storage
}

func (noSignedURLStorage) GetSignedURLs(context.Context, *coordinator.URLSigningRequest) (
	*coordinator.URLSigningResponse, error) {

	return nil, nil
}

type googleStorage struct {
	// Storage is the base storage.Storage instance.
	storage.Storage
	// svc is the services instance that created this.
	svc *flexServicesInst

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
	si.Storage.Close()
	si.gs.Close()
}

func (si *googleStorage) GetSignedURLs(c context.Context, req *coordinator.URLSigningRequest) (
	*coordinator.URLSigningResponse, error) {

	info, err := si.svc.Signer.ServiceInfo(c)
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
			_, signedBytes, err := si.svc.Signer.SignBytes(c, b)
			return signedBytes, err
		},
		Method:  "GET",
		Expires: resp.Expiration,
	}

	doSign := func(path gs.Path) (string, error) {
		url, err := gcst.SignedURL(path.Bucket(), path.Filename(), &opts)
		if err != nil {
			return "", errors.Annotate(err, "").InternalReason(
				"failed to sign URL: bucket(%s)/filename(%s)", path.Bucket(), path.Filename()).Err()
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
