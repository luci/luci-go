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

package service

import (
	"context"
	"flag"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/luci/appengine/gaesettings"
	commonAuth "go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	gcps "go.chromium.org/luci/common/gcloud/pubsub"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gkelogger"
	"go.chromium.org/luci/common/logging/teelogger"
	"go.chromium.org/luci/common/runtime/profiling"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/target"
	cfglib "go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/filesystem"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/backend/client"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/config/server/cfgclient/textproto"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/logdog/api/config/svcconfig"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/storage/bigtable"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/logdog/server/retryServicesClient"
	"go.chromium.org/luci/logdog/server/service/config"
	serverAuth "go.chromium.org/luci/server/auth"
	serverCaching "go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/settings"

	"go.chromium.org/gae/impl/cloud"

	cloudBT "cloud.google.com/go/bigtable"
	"cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/datastore"
	"cloud.google.com/go/pubsub"

	"golang.org/x/oauth2"
	"google.golang.org/api/option"
)

var (
	// ErrInvalidConfig is an error that is returned when the supplied
	// configuration is invalid.
	ErrInvalidConfig = errors.New("invalid configuration")

	// CoordinatorScopes is the set of OAuth2 scopes to use for the Coordinator
	// client.
	CoordinatorScopes = []string{
		commonAuth.OAuthScopeEmail,
	}
)

// projectConfigCacheDuration is the amount of time to cache a project's
// configuration before reloading.
const (
	projectConfigCacheDuration = 30 * time.Minute

	// minAuthTokenLifetime is the amount of time that an access token has before
	// expiring.
	minAuthTokenLifetime = 2 * time.Minute
)

// Service is a base class full of common LogDog service application parameters.
type Service struct {
	// Name is the name of this service. It is used for logging, metrics, and
	// user agent string generation.
	//
	// If empty, a service name will be inferred from the command-line arguments.
	Name string

	// DefaultAuthOptions provide default values for authentication related
	// options (most notably SecretsDir: a directory with token cache).
	DefaultAuthOptions commonAuth.Options

	// Flags is the set of flags that will be used by the Service.
	Flags flag.FlagSet

	shutdownFunc atomic.Value

	loggingFlags log.Config
	authFlags    authcli.Flags
	tsMonFlags   tsmon.Flags
	profiler     profiling.Profiler

	coordinatorHost     string
	coordinatorInsecure bool
	useDatastoreConfig  bool

	// onGCE is true if we're on GCE. We probe this once during Run.
	onGCE        bool
	hasDatastore bool

	// killCheckInterval is the amount of time in between service configuration
	// checks. If set, this service will periodically reload its service
	// configuration. If that configuration has changed, the service will kill
	// itself.
	//
	// Since, in production, this is running under an execution harness such as
	// Kubernetes, the service will restart and load the new configuration. This
	// is easier than implementing in-process configuration updating.
	killCheckInterval clockflag.Duration
	// testConfigFilePath is the path to a local configuration service filesystem
	// (impl/filesystem) root. This is used for testing.
	testConfigFilePath string
	// serviceConfig is the cached service configuration.
	serviceConfig svcconfig.Config
	configCache   config.MessageCache

	// serviceID is the cloud project ID, which is also this service's unique
	// ID. This can be specified by flag or, if on GCE, will automatically be
	// probed from metadata.
	serviceID string

	coord logdog.ServicesClient

	// authCache is a cache of instantiated Authenticator instances, keyed on
	// sorted NULL-delimited scope strings (see authenticatorForScopes).
	authCacheLock sync.RWMutex
	authCache     map[string]*commonAuth.Authenticator
}

// Run performs service-wide initialization and invokes the specified run
// function.
func (s *Service) Run(c context.Context, f func(context.Context) error) {
	// Log to Stdout using JSON log lines.
	c = teelogger.Use(c, gkelogger.GetFactory(os.Stdout))

	// If a service name isn't specified, default to the base of the current
	// executable.
	if s.Name == "" {
		s.Name = filepath.Base(os.Args[0])
	}
	s.useDatastoreConfig = true // If available, use datastore config.

	rc := 0
	if err := s.runImpl(c, f); err != nil {
		log.WithError(err).Errorf(c, "Application exiting with error.")
		rc = 1
	}
	os.Exit(rc)
}

func (s *Service) runImpl(c context.Context, f func(context.Context) error) error {
	// Set log level to Info for initial setup. This will be overridden when the
	// flags are parsed.
	c = log.SetLevel(c, log.Info)

	// Probe our environment for default values. Set log level to Info for this
	// b/c we haven't parsed flags yet.
	s.probeGCEEnvironment(c)

	// Install service flags and parse.
	s.addFlags(c, &s.Flags)
	if err := s.Flags.Parse(os.Args[1:]); err != nil {
		log.WithError(err).Errorf(c, "Failed to parse command-line.")
		return err
	}

	// Install logging configuration.
	c = s.loggingFlags.Set(c)

	if err := s.profiler.Start(); err != nil {
		return errors.Annotate(err, "failed to start profiler").Err()
	}
	defer s.profiler.Stop()

	// Cancel our Context after we're done our run loop.
	c, cancelFunc := context.WithCancel(c)
	defer cancelFunc()

	// Validate the runtime environment.
	if s.serviceID == "" {
		return errors.New("no service ID was configured (-service-id)")
	}

	// Install our authentication service.
	c = s.withAuthService(c)

	// Install a cloud datastore client. This is non-fatal if it fails.
	dsClient, err := s.initDatastoreClient(c)
	if err == nil {
		defer dsClient.Close()

		ccfg := cloud.Config{
			DS:        dsClient,
			ProjectID: s.serviceID,
		}
		c = ccfg.Use(c, nil)
		c = settings.Use(c, settings.New(gaesettings.Storage{}))

		s.hasDatastore = s.useDatastoreConfig
		log.Debugf(c, "Enabled cloud datastore access.")
	} else {
		log.WithError(err).Warningf(c, "Failed to create cloud datastore client.")
	}

	// Install process-wide cache.
	c = serverCaching.WithEmptyProcessCache(c)

	// Configure our signal handler. It will listen for terminating signals and
	// issue a shutdown signal if one is received.
	signalC := make(chan os.Signal)
	go func(c context.Context) {
		hasShutdownAlready := false
		for sig := range signalC {
			if !hasShutdownAlready {
				hasShutdownAlready = true

				log.Warningf(log.SetField(c, "signal", sig), "Received close signal. Send again to terminate immediately.")
				s.shutdown()
				continue
			}

			// No shutdown function registered; just exit immediately.
			s.shutdownImmediately()
			panic("never reached")
		}
	}(c)
	signal.Notify(signalC, os.Interrupt)
	defer func() {
		signal.Stop(signalC)
		close(signalC)
	}()

	// Initialize our tsmon library.
	c = tsmon.WithState(c, tsmon.NewState())

	if err := tsmon.InitializeFromFlags(c, &s.tsMonFlags); err != nil {
		log.WithError(err).Warningf(c, "Failed to initialize monitoring; will continue without metrics.")
	}
	defer tsmon.Shutdown(c)

	// Initialize our Client instantiations.
	if s.coord, err = s.initCoordinatorClient(c); err != nil {
		log.WithError(err).Errorf(c, "Failed to setup Coordinator client.")
		return err
	}

	// Initialize and install our config service and caching layers, and load our
	// initial service config.
	if err := s.initConfig(&c); err != nil {
		log.WithError(err).Errorf(c, "Failed to setup configuration.")
		return err
	}

	// Clear our shutdown function on termination.
	defer s.SetShutdownFunc(nil)

	// Run main service function.
	return f(c)
}

func (s *Service) addFlags(c context.Context, fs *flag.FlagSet) {
	// Initialize logging flags.
	s.loggingFlags.Level = log.Warning
	s.loggingFlags.AddFlags(fs)

	// Initialize tsmon flags.
	s.tsMonFlags = tsmon.NewFlags()
	s.tsMonFlags.Flush = tsmon.FlushAuto
	s.tsMonFlags.Target.TargetType = target.TaskType
	s.tsMonFlags.Target.TaskServiceName = s.serviceID
	s.tsMonFlags.Target.TaskJobName = s.Name
	s.tsMonFlags.Register(fs)

	// Initialize auth flags.
	s.authFlags.Register(fs, s.DefaultAuthOptions)

	// Initialize profiling flags.
	s.profiler.AddFlags(fs)

	fs.StringVar(&s.serviceID, "service-id", s.serviceID,
		"Specify the service ID that this instance is supporting. If empty, the service ID "+
			"will attempt to be resolved by probing the local environment. This probably will match the "+
			"App ID of the Coordinator.")
	fs.StringVar(&s.coordinatorHost, "coordinator", s.coordinatorHost,
		"The Coordinator service's [host][:port].")
	fs.BoolVar(&s.coordinatorInsecure, "coordinator-insecure", s.coordinatorInsecure,
		"Connect to Coordinator over HTTP (instead of HTTPS).")
	fs.Var(&s.killCheckInterval, "config-kill-interval",
		"If non-zero, poll for configuration changes and kill the application if one is detected.")
	fs.StringVar(&s.testConfigFilePath, "test-config-file-path", s.testConfigFilePath,
		"(Testing) If set, load configuration from a local filesystem rooted here.")
	fs.BoolVar(&s.useDatastoreConfig, "use-datastore-config", s.useDatastoreConfig,
		"Enable/Disable loading configuration directly from datastore cache.")
}

// probeGCEEnvironment fills in any parameters that can be probed from Google
// Compute Engine metadata.
//
// If we're not running on GCE, this will do nothing. It is non-fatal if any
// given GCE field fails to be probed.
func (s *Service) probeGCEEnvironment(c context.Context) {
	s.onGCE = metadata.OnGCE()
	if !s.onGCE {
		log.Infof(c, "Not on GCE.")
		return
	}

	// Determine our service ID from metadata. The service ID will equal the cloud
	// project ID.
	if s.serviceID == "" {
		var err error
		if s.serviceID, err = metadata.ProjectID(); err != nil {
			log.WithError(err).Warningf(c, "Failed to probe GCE project ID.")
		}

		log.Fields{
			"serviceID": s.serviceID,
		}.Infof(c, "Probed GCE service ID.")
	}
}

func (s *Service) initDatastoreClient(c context.Context) (*datastore.Client, error) {
	ts, err := serverAuth.GetTokenSource(
		c, serverAuth.AsSelf,
		serverAuth.WithScopes(datastore.ScopeDatastore))
	if err != nil {
		return nil, err
	}
	return datastore.NewClient(c, s.serviceID,
		option.WithUserAgent(s.getUserAgent()),
		option.WithTokenSource(ts))
}

func (s *Service) initCoordinatorClient(c context.Context) (logdog.ServicesClient, error) {
	if s.coordinatorHost == "" {
		log.Errorf(c, "Missing Coordinator URL (-coordinator).")
		return nil, ErrInvalidConfig
	}

	transport, err := serverAuth.GetRPCTransport(c, serverAuth.AsSelf, serverAuth.WithScopes(CoordinatorScopes...))
	if err != nil {
		log.Errorf(c, "Failed to create authenticated transport for Coordinator client.")
		return nil, err
	}

	prpcClient := prpc.Client{
		C: &http.Client{
			Transport: transport,
		},
		Host:    s.coordinatorHost,
		Options: prpc.DefaultOptions(),
	}
	if s.coordinatorInsecure {
		prpcClient.Options.Insecure = true
	}
	sc := logdog.NewServicesPRPCClient(&prpcClient)

	// Wrap the resulting client in a retry harness.
	return retryServicesClient.New(sc, nil), nil
}

func (s *Service) initConfig(c *context.Context) error {
	// Set up our in-memory config object cache.
	s.configCache.Lifetime = projectConfigCacheDuration

	// Start to build our backend caching options.
	opts := config.CacheOptions{
		CacheExpiration: projectConfigCacheDuration,
	}

	// If a testConfigFilePath was specified, use a mock configuration service
	// that loads from a local file.
	var p client.Provider
	if s.testConfigFilePath == "" {
		ccfg, err := s.coord.GetConfig(*c, &empty.Empty{})
		if err != nil {
			return err
		}

		// Determine our config service host.
		//
		// Older Coordinator instances may provide the full URL instead of the host,
		// in which case we will extract the host from the URL.
		host := ccfg.ConfigServiceHost
		if host == "" {
			if ccfg.ConfigServiceUrl == "" {
				return errors.New("coordinator does not specify a config service")
			}
			u, err := url.Parse(ccfg.ConfigServiceUrl)
			if err != nil {
				return errors.Annotate(err, "failed to parse config service URL").Err()
			}
			host = u.Host
		}

		if ccfg.ConfigSet == "" {
			return errors.New("coordinator does not specify a config set")
		}

		log.Fields{
			"host": host,
		}.Debugf(*c, "Using remote configuration service client.")
		p = &client.RemoteProvider{
			Host: host,
		}

		// If using a remote config provider, enable datastore access and caching.
		opts.DatastoreCacheAvailable = s.hasDatastore
	} else {
		// Test / Local: use filesystem config path.
		ci, err := filesystem.New(s.testConfigFilePath)
		if err != nil {
			return err
		}
		p = &testconfig.Provider{Base: ci}
	}

	// Add config caching layers.
	*c = opts.WrapBackend(*c, &client.Backend{
		Provider: p,
	})

	// Load our service configuration.
	var meta cfglib.Meta
	cset, path := s.ServiceConfigPath()
	if err := cfgclient.Get(*c, cfgclient.AsService, cset, path, textproto.Message(&s.serviceConfig), &meta); err != nil {
		return errors.Annotate(err, "failed to load service config").Err()
	}

	// Create a poller for our service config.
	if s.killCheckInterval > 0 {
		pollerC, pollerCancelFunc := context.WithCancel(*c)

		poller := config.ChangePoller{
			ConfigSet: cset,
			Path:      path,
			Period:    time.Duration(s.killCheckInterval),
			OnChange: func() {
				// When a configuration change is detected, stop future polling and call
				// our shutdown function.
				pollerCancelFunc()
				s.shutdown()
			},
			ContentHash: meta.ContentHash,
		}
		go poller.Run(pollerC)
	}
	return nil
}

// ServiceConfigPath returns the ConfigSet and path to the current service's
// configuration.
func (s *Service) ServiceConfigPath() (cfglib.Set, string) {
	return cfglib.ServiceSet(s.serviceID), svcconfig.ServiceConfigPath
}

// ServiceConfig returns the configuration data for the current service.
func (s *Service) ServiceConfig() *svcconfig.Config { return &s.serviceConfig }

// ProjectConfigPath returns the ConfigSet and path to the current service's
// project configuration for proj.
func (s *Service) ProjectConfigPath(proj types.ProjectName) (cfglib.Set, string) {
	return cfglib.ProjectSet(string(proj)), svcconfig.ProjectConfigPath(s.serviceID)
}

// ProjectConfig returns the current service's project configuration for proj.
func (s *Service) ProjectConfig(c context.Context, proj types.ProjectName) (*svcconfig.ProjectConfig, error) {
	cset, path := s.ProjectConfigPath(proj)

	var pcfg svcconfig.ProjectConfig
	msg, err := s.configCache.Get(c, cset, path, &pcfg)
	if err != nil {
		return nil, errors.Annotate(err, "failed to load project config from %s.%s", cset, path).Err()
	}
	return msg.(*svcconfig.ProjectConfig), nil
}

// SetShutdownFunc sets the service shutdown function.
func (s *Service) SetShutdownFunc(f func()) {
	s.shutdownFunc.Store(f)
}

func (s *Service) shutdown() {
	v := s.shutdownFunc.Load()
	if f, ok := v.(func()); ok {
		f()
	} else {
		s.shutdownImmediately()
	}
}

func (s *Service) shutdownImmediately() {
	os.Exit(1)
}

// Coordinator returns the cached Coordinator client.
func (s *Service) Coordinator() logdog.ServicesClient {
	return s.coord
}

// ServiceID returns the service ID.
//
// This is synonymous with the cloud "project ID" and the AppEngine "app ID".
func (s *Service) ServiceID() string {
	return s.serviceID
}

// IntermediateStorage instantiates the configured intermediate Storage
// instance.
//
// If "rw" is true, Read/Write access will be requested. Otherwise, read-only
// access will be requested.
func (s *Service) IntermediateStorage(c context.Context, rw bool) (storage.Storage, error) {
	cfg := s.ServiceConfig()
	if cfg.GetStorage() == nil {
		log.Errorf(c, "Missing storage configuration.")
		return nil, ErrInvalidConfig
	}

	btcfg := cfg.GetStorage().GetBigtable()
	if btcfg == nil {
		log.Errorf(c, "Missing BigTable storage configuration")
		return nil, ErrInvalidConfig
	}

	// Determine our scopes.
	scopes := bigtable.StorageReadOnlyScopes
	if rw {
		scopes = bigtable.StorageScopes
	}

	// Initialize RPC credentials.
	ts, err := serverAuth.GetTokenSource(c, serverAuth.AsSelf, serverAuth.WithScopes(scopes...))
	if err != nil {
		return nil, err
	}
	client, err := cloudBT.NewClient(c, btcfg.Project, btcfg.Instance,
		option.WithUserAgent(s.getUserAgent()), option.WithTokenSource(ts))
	if err != nil {
		return nil, err
	}
	return &bigtable.Storage{
		Client:   client,
		LogTable: btcfg.LogTableName,
	}, nil
}

// GSClient returns an authenticated Google Storage client instance.
func (s *Service) GSClient(c context.Context) (gs.Client, error) {
	// Get an Authenticator bound to the token scopes that we need for
	// authenticated Cloud Storage access.
	transport, err := serverAuth.GetRPCTransport(c, serverAuth.AsSelf, serverAuth.WithScopes(gs.ReadWriteScopes...))
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create authenticated transport for Google Storage client.")
		return nil, err
	}

	client, err := gs.NewProdClient(c, transport)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Google Storage client.")
		return nil, err
	}
	return client, nil
}

// PubSubSubscriberClient returns a Pub/Sub client instance that is
// authenticated with Pub/Sub subscriber scopes.
func (s *Service) PubSubSubscriberClient(c context.Context, projectID string) (*pubsub.Client, error) {
	ts, err := serverAuth.GetTokenSource(
		c, serverAuth.AsSelf,
		serverAuth.WithScopes(gcps.SubscriberScopes...))
	if err != nil {
		return nil, err
	}
	return pubsub.NewClient(c, projectID,
		option.WithUserAgent(s.getUserAgent()),
		option.WithTokenSource(ts))
}

func (s *Service) unauthenticatedTransport() http.RoundTripper {
	smt := serviceModifyingTransport{
		userAgent: s.getUserAgent(),
	}
	return smt.roundTripper(nil)
}

func (s *Service) getUserAgent() string { return s.Name + " / " + s.serviceID }

// withAuthService configures service-wide authentication and installs it into
// the supplied Context.
func (s *Service) withAuthService(c context.Context) context.Context {
	return serverAuth.Initialize(c, &serverAuth.Config{
		DBProvider: nil, // We don't need to store an auth DB.
		Signer:     nil, // We don't need to sign anything.
		AccessTokenProvider: func(ic context.Context, scopes []string) (*oauth2.Token, error) {
			// Create a new Authenticator for the supplied scopes.
			//
			// Pass our outer Context, since we don't want the cached Authenticator
			// instance to be permanently bound to the inner Context.
			a, err := s.authenticatorForScopes(c, scopes)
			if err != nil {
				log.Fields{
					"scopes":     scopes,
					log.ErrorKey: err,
				}.Errorf(c, "Failed to create authenticator.")
				return nil, err
			}
			tok, err := a.GetAccessToken(minAuthTokenLifetime)
			if err != nil {
				log.Fields{
					"scopes":     scopes,
					log.ErrorKey: err,
				}.Errorf(c, "Failed to mint access token.")
			}
			return tok, err
		},
		AnonymousTransport: func(ic context.Context) http.RoundTripper {
			return s.unauthenticatedTransport()
		},
	})
}

func (s *Service) authenticatorForScopes(c context.Context, scopes []string) (*commonAuth.Authenticator, error) {
	sort.Strings(scopes)
	key := strings.Join(scopes, "\x00")

	// First, check holding read lock.
	s.authCacheLock.RLock()
	a := s.authCache[key]
	s.authCacheLock.RUnlock()

	if a != nil {
		return a, nil
	}

	// No authenticator yet, check again with write lock.
	s.authCacheLock.Lock()
	defer s.authCacheLock.Unlock()

	if a = s.authCache[key]; a != nil {
		// One was created in between locking!
		return a, nil
	}

	// Create a new Authenticator.
	authOpts, err := s.authFlags.Options()
	if err != nil {
		return nil, ErrInvalidConfig
	}
	authOpts.Scopes = append([]string(nil), scopes...)
	authOpts.Transport = s.unauthenticatedTransport()

	a = commonAuth.NewAuthenticator(c, commonAuth.SilentLogin, authOpts)
	if s.authCache == nil {
		s.authCache = make(map[string]*commonAuth.Authenticator)
	}
	s.authCache[key] = a
	return a, nil
}
