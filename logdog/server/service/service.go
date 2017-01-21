// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package service

import (
	"flag"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/luci/luci-go/appengine/gaesettings"
	"github.com/luci/luci-go/client/authcli"
	commonAuth "github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/clock/clockflag"
	"github.com/luci/luci-go/common/config/impl/filesystem"
	"github.com/luci/luci-go/common/data/caching/proccache"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/gcloud/gs"
	gcps "github.com/luci/luci-go/common/gcloud/pubsub"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/tsmon"
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/grpc/prpc"
	"github.com/luci/luci-go/logdog/api/config/svcconfig"
	"github.com/luci/luci-go/logdog/api/endpoints/coordinator/services/v1"
	"github.com/luci/luci-go/logdog/common/storage"
	"github.com/luci/luci-go/logdog/common/storage/bigtable"
	"github.com/luci/luci-go/logdog/server/retryServicesClient"
	"github.com/luci/luci-go/logdog/server/service/config"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"
	"github.com/luci/luci-go/luci_config/server/cfgclient"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/client"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/testconfig"
	"github.com/luci/luci-go/luci_config/server/cfgclient/textproto"
	serverAuth "github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/settings"

	"github.com/luci/gae/impl/cloud"

	"cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/datastore"
	"cloud.google.com/go/pubsub"
	"golang.org/x/net/context"
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

	// authCacheSize is the maximum number of elements to store in the auth
	// global cache LRU.
	//
	// We don't expect to load too many different authentication tokens, so a
	// relatively low number should be fine here.
	authCacheSize = 128
)

// Service is a base class full of common LogDog service application parameters.
type Service struct {
	// Name is the name of this service. It is used for logging, metrics, and
	// user agent string generation.
	//
	// If empty, a service name will be inferred from the command-line arguments.
	Name string
	// Flags is the set of flags that will be used by the Service.
	Flags flag.FlagSet

	shutdownFunc atomic.Value

	loggingFlags log.Config
	authFlags    authcli.Flags
	tsMonFlags   tsmon.Flags

	coordinatorHost     string
	coordinatorInsecure bool
	cpuProfilePath      string
	heapProfilePath     string

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
	c = gologger.StdConfig.Use(c)

	// If a service name isn't specified, default to the base of the current
	// executable.
	if s.Name == "" {
		s.Name = filepath.Base(os.Args[0])
	}

	rc := 0
	if err := s.runImpl(c, f); err != nil {
		log.WithError(err).Errorf(c, "Application exiting with error.")
		rc = 1
	}
	os.Exit(rc)
}

func (s *Service) runImpl(c context.Context, f func(context.Context) error) error {
	// Probe our environment for default values.
	s.probeGCEEnvironment(c)

	// Install service flags and parse.
	s.addFlags(c, &s.Flags)
	if err := s.Flags.Parse(os.Args[1:]); err != nil {
		log.WithError(err).Errorf(c, "Failed to parse command-line.")
		return err
	}

	// Install logging configuration.
	c = s.loggingFlags.Set(c)

	if p := s.cpuProfilePath; p != "" {
		fd, err := os.Create(p)
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"path":       p,
			}.Errorf(c, "Failed to create CPU profile output file.")
			return err
		}
		defer fd.Close()

		pprof.StartCPUProfile(fd)
		defer pprof.StopCPUProfile()
	}

	if p := s.heapProfilePath; p != "" {
		defer func() {
			fd, err := os.Create(p)
			if err != nil {
				log.Fields{
					log.ErrorKey: err,
					"path":       p,
				}.Warningf(c, "Failed to create heap profile output file.")
				return
			}
			defer fd.Close()

			if err := pprof.WriteHeapProfile(fd); err != nil {
				log.Fields{
					log.ErrorKey: err,
					"path":       p,
				}.Warningf(c, "Failed to write heap profile.")
			}
		}()
	}

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
			DS: dsClient,
		}
		c = ccfg.Use(c)
		c = settings.Use(c, settings.New(gaesettings.Storage{}))

		s.hasDatastore = true
		log.Debugf(c, "Enabled cloud datastore access.")
	} else {
		log.WithError(err).Warningf(c, "Failed to create cloud datastore client.")
	}

	// Install a process-wide cache.
	c = proccache.Use(c, &proccache.Cache{})

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
	s.tsMonFlags.Target.TaskJobName = s.Name
	s.tsMonFlags.Register(fs)

	s.authFlags.Register(fs, commonAuth.Options{})

	fs.StringVar(&s.serviceID, "service-id", "",
		"Specify the service ID that this instance is supporting. If empty, the service ID "+
			"will attempt to be resolved by probing the local environment. This probably will match the "+
			"App ID of the Coordinator.")
	fs.StringVar(&s.coordinatorHost, "coordinator", "",
		"The Coordinator service's [host][:port].")
	fs.BoolVar(&s.coordinatorInsecure, "coordinator-insecure", false,
		"Connect to Coordinator over HTTP (instead of HTTPS).")
	fs.StringVar(&s.cpuProfilePath, "cpu-profile-path", "",
		"If supplied, enable CPU profiling and write the profile here.")
	fs.StringVar(&s.heapProfilePath, "heap-profile-path", "",
		"If supplied, enable CPU profiling and write the profile here.")
	fs.Var(&s.killCheckInterval, "config-kill-interval",
		"If non-zero, poll for configuration changes and kill the application if one is detected.")
	fs.StringVar(&s.testConfigFilePath, "test-config-file-path", "",
		"(Testing) If set, load configuration from a local filesystem rooted here.")
}

// probeGCEEnvironment fills in any parameters that can be probed from Google
// Compute Engine metadata.
//
// If we're not running on GCE, this will do nothing. It is non-fatal if any
// given GCE field fails to be probed.
func (s *Service) probeGCEEnvironment(c context.Context) {
	s.onGCE = metadata.OnGCE()
	if !s.onGCE {
		return
	}

	// Determine our service ID from metadata. The service ID will equal the cloud
	// project ID.
	if s.serviceID == "" {
		var err error
		if s.serviceID, err = metadata.ProjectID(); err != nil {
			log.WithError(err).Warningf(c, "Failed to probe GCE project ID.")
		}
	}
}

func (s *Service) initDatastoreClient(c context.Context) (*datastore.Client, error) {
	return datastore.NewClient(c, s.serviceID,
		option.WithUserAgent(s.getUserAgent()),
		option.WithTokenSource(serverAuth.GetTokenSourceAsSelf(c, datastore.ScopeDatastore)))
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
		ccfg, err := s.coord.GetConfig(*c, &google.Empty{})
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
				return errors.Annotate(err).Reason("failed to parse config service URL").Err()
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
	var meta cfgclient.Meta
	cset, path := s.ServiceConfigPath()
	if err := cfgclient.Get(*c, cfgclient.AsService, cset, path, textproto.Message(&s.serviceConfig), &meta); err != nil {
		return errors.Annotate(err).Reason("failed to load service config").Err()
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
func (s *Service) ServiceConfigPath() (cfgtypes.ConfigSet, string) {
	return cfgtypes.ServiceConfigSet(s.serviceID), svcconfig.ServiceConfigPath
}

// ServiceConfig returns the configuration data for the current service.
func (s *Service) ServiceConfig() *svcconfig.Config { return &s.serviceConfig }

// ProjectConfigPath returns the ConfigSet and path to the current service's
// project configuration for proj.
func (s *Service) ProjectConfigPath(proj cfgtypes.ProjectName) (cfgtypes.ConfigSet, string) {
	return cfgtypes.ProjectConfigSet(proj), svcconfig.ProjectConfigPath(s.serviceID)
}

// ProjectConfig returns the current service's project configuration for proj.
func (s *Service) ProjectConfig(c context.Context, proj cfgtypes.ProjectName) (*svcconfig.ProjectConfig, error) {
	cset, path := s.ProjectConfigPath(proj)

	var pcfg svcconfig.ProjectConfig
	msg, err := s.configCache.Get(c, cset, path, &pcfg)
	if err != nil {
		return nil, errors.Annotate(err).Reason("failed to load project config from %(cset)s.%(path)s").
			D("cset", cset).D("path", path).Err()
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
func (s *Service) IntermediateStorage(c context.Context) (storage.Storage, error) {
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

	// Initialize RPC credentials.
	bt, err := bigtable.New(c, bigtable.Options{
		Project:  btcfg.Project,
		Instance: btcfg.Instance,
		LogTable: btcfg.LogTableName,
		ClientOptions: []option.ClientOption{
			option.WithUserAgent(s.getUserAgent()),
			option.WithTokenSource(serverAuth.GetTokenSourceAsSelf(c, bigtable.StorageScopes...)),
		},
	})
	if err != nil {
		return nil, err
	}
	return bt, nil
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
	return pubsub.NewClient(c, projectID,
		option.WithUserAgent(s.getUserAgent()),
		option.WithTokenSource(serverAuth.GetTokenSourceAsSelf(c, gcps.SubscriberScopes...)))
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
	return serverAuth.SetConfig(c, serverAuth.Config{
		DBProvider: nil, // We don't need to store an auth DB.
		Signer:     nil, // We don't need to sign anything.
		AccessTokenProvider: func(ic context.Context, scopes []string) (commonAuth.Token, error) {
			// Create a new Authenticator for the supplied scopes.
			//
			// Pass our outer Context, since we don't want the cached Authenticator
			// instance to be permanently bound to the inner Context.
			a, err := s.authenticatorForScopes(c, scopes)
			if err != nil {
				return commonAuth.Token{}, err
			}
			return a.GetAccessToken(minAuthTokenLifetime)
		},
		AnonymousTransport: func(ic context.Context) http.RoundTripper {
			return s.unauthenticatedTransport()
		},
		Cache: serverAuth.MemoryCache(authCacheSize),
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
