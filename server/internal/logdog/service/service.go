// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"sync"

	"github.com/luci/luci-go/client/authcli"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/auth"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"github.com/luci/luci-go/common/prpc"
	"github.com/luci/luci-go/server/internal/logdog/config"
	"github.com/luci/luci-go/server/internal/logdog/retryServicesClient"
	"github.com/luci/luci-go/server/logdog/storage"
	"github.com/luci/luci-go/server/logdog/storage/bigtable"
	"golang.org/x/net/context"
	"google.golang.org/cloud"
)

var (
	// ErrInvalidConfig is an error that is returned when the supplied
	// configuration is invalid.
	ErrInvalidConfig = errors.New("invalid configuration")

	// CoordinatorScopes is the set of OAuth2 scopes to use for the Coordinator
	// client.
	CoordinatorScopes = []string{
		auth.OAuthScopeEmail,
	}
)

// Service is a base class full of common LogDog service application parameters.
type Service struct {
	context.Context

	// UserAgent is the user agent string that will be used for service
	// communication.
	UserAgent string

	// ShutdownFunc, if not nil, is a function that will be called when a shutdown
	// signal is received.
	ShutdownFunc func()

	// topCancelFunc is the Context cancel function for the top-level application
	// Context.
	topCancelFunc func()

	// shutdownMu protects the shutdown variables.
	shutdownMu    sync.Mutex
	shutdownFunc  func()
	shutdownCount int32

	loggingFlags log.Config
	authFlags    authcli.Flags
	configFlags  config.Flags

	coordinatorHost           string
	coordinatorInsecure       bool
	storageCredentialJSONPath string

	coord  services.ServicesClient
	config *config.Manager
}

// New instantiates a new Service.
func New(c context.Context) *Service {
	c, cancelFunc := context.WithCancel(c)
	c = gologger.Use(c)

	return &Service{
		Context:       c,
		topCancelFunc: cancelFunc,
	}
}

// AddFlags adds standard service flags to the supplied FlagSet.
func (s *Service) AddFlags(fs *flag.FlagSet) {
	s.loggingFlags.AddFlags(fs)
	s.authFlags.Register(fs, auth.Options{
		Context: s,
		Logger:  log.Get(s),
	})
	s.configFlags.AddToFlagSet(fs)

	fs.StringVar(&s.coordinatorHost, "coordinator", "",
		"The Coordinator service's [host][:port].")
	fs.BoolVar(&s.coordinatorInsecure, "coordinator-insecure", false,
		"Connect to Coordinator over HTTP (instead of HTTPS).")
	fs.StringVar(&s.storageCredentialJSONPath, "storage-credential-json-path", "",
		"If supplied, the path of a JSON credential file to load and use for storage operations.")
}

// Run loads the Service's base runtime and invokes the specified run function.
func (s *Service) Run(f func() error) error {
	s.Context = s.loggingFlags.Set(s.Context)

	// Configure our signal handler. It will listen for terminating signals and
	// issue a shutdown signal if one is received.
	signalC := make(chan os.Signal)
	go func() {
		for sig := range signalC {
			s.Shutdown()
			log.Warningf(log.SetField(s, "signal", sig), "Received close signal. Send again to terminate immediately.")
		}
	}()
	signal.Notify(signalC, os.Interrupt)
	defer func() {
		signal.Stop(signalC)
		close(signalC)
	}()

	// Initialize our Client instantiations.
	var err error
	s.coord, err = s.initCoordinatorClient()
	if err != nil {
		log.Errorf(log.SetError(s, err), "Failed to setup Coordinator client.")
		return err
	}

	s.config, err = s.initConfig()
	if err != nil {
		log.Errorf(log.SetError(s, err), "Failed to setup configuration.")
		return err
	}

	return f()
}

func (s *Service) initCoordinatorClient() (services.ServicesClient, error) {
	if s.coordinatorHost == "" {
		log.Errorf(s, "Missing Coordinator URL (-coordinator).")
		return nil, ErrInvalidConfig
	}

	httpClient, err := s.AuthenticatedClient(func(o *auth.Options) {
		o.Scopes = CoordinatorScopes
	})
	if err != nil {
		log.Errorf(log.SetError(s, err), "Failed to create authenticated client.")
		return nil, err
	}

	prpcClient := prpc.Client{
		C:       httpClient,
		Host:    s.coordinatorHost,
		Options: prpc.DefaultOptions(),
	}
	if s.coordinatorInsecure {
		prpcClient.Options.Insecure = true
	}
	sc := services.NewServicesPRPCClient(&prpcClient)

	// Wrap the resulting client in a retry harness.
	return retryServicesClient.New(sc, nil), nil
}

func (s *Service) initConfig() (*config.Manager, error) {
	rt, err := s.AuthenticatedTransport(nil)
	if err != nil {
		log.Errorf(log.SetError(s, err), "Failed to create config client.")
		return nil, err
	}

	s.configFlags.RoundTripper = rt
	o, err := s.configFlags.CoordinatorOptions(s, s.coord)
	if err != nil {
		log.Errorf(log.SetError(s, err), "Failed to load configuration parameters.")
		return nil, err
	}
	o.KillFunc = s.Shutdown

	return config.NewManager(s, *o)
}

// Shutdown issues a shutdown signal to the service.
func (s *Service) Shutdown() {
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()

	if s.shutdownCount > 0 {
		os.Exit(1)
	}
	s.shutdownCount++

	if f := s.shutdownFunc; f != nil {
		f()
	} else {
		s.topCancelFunc()
	}
}

// SetShutdownFunc sets the service shutdown function.
func (s *Service) SetShutdownFunc(f func()) {
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()
	s.shutdownFunc = f
}

// Config returns the cached service configuration.
func (s *Service) Config() *svcconfig.Config {
	return s.config.Config()
}

// Coordinator returns the cached Coordinator client.
func (s *Service) Coordinator() services.ServicesClient {
	return s.coord
}

// Storage instantiates the configured Storage instance.
func (s *Service) Storage() (storage.Storage, error) {
	cfg := s.config.Config()
	if cfg.GetStorage() == nil {
		log.Errorf(s, "Missing storage configuration.")
		return nil, ErrInvalidConfig
	}

	btcfg := cfg.GetStorage().GetBigtable()
	if btcfg == nil {
		log.Errorf(s, "Missing BigTable storage configuration")
		return nil, ErrInvalidConfig
	}

	// Initialize Storage authentication.
	a, err := s.Authenticator(func(o *auth.Options) {
		o.Scopes = bigtable.StorageScopes
		if s.storageCredentialJSONPath != "" {
			o.ServiceAccountJSONPath = s.storageCredentialJSONPath
		}
	})
	if err != nil {
		log.Errorf(log.SetError(s, err), "Failed to create BigTable Authenticator.")
		return nil, err
	}

	bt, err := bigtable.New(s, bigtable.Options{
		Project:  btcfg.Project,
		Zone:     btcfg.Zone,
		Cluster:  btcfg.Cluster,
		LogTable: btcfg.LogTableName,
		ClientOptions: []cloud.ClientOption{
			cloud.WithTokenSource(a.TokenSource()),
		},
	})
	if err != nil {
		log.Errorf(log.SetError(s, err), "Failed to create BigTable instance.")
		return nil, err
	}
	return bt, nil
}

// Authenticator returns an Authenticator instance. The Authenticator is
// configured from a base set of Authenticator Options.
//
// An optional permutation functon can be provided to modify those Options
// before the Authenticator is created.
func (s *Service) Authenticator(f func(o *auth.Options)) (*auth.Authenticator, error) {
	authOpts, err := s.authFlags.Options()
	if err != nil {
		log.Errorf(log.SetError(s, err), "Failed to create authenticator options.")
		return nil, ErrInvalidConfig
	}
	if f != nil {
		f(&authOpts)
	}
	return auth.NewAuthenticator(auth.SilentLogin, authOpts), nil
}

// AuthenticatedTransport returns an authenticated http.RoundTripper transport.
// The transport is configured from a base set of Authenticator Options.
//
// An optional permutation functon can be provided to modify those Options
// before the Authenticator is created.
func (s *Service) AuthenticatedTransport(f func(o *auth.Options)) (http.RoundTripper, error) {
	a, err := s.Authenticator(f)
	if err != nil {
		return nil, err
	}
	return a.Transport()
}

// AuthenticatedClient returns an authenticated http.Client. The Client is
// configured from a base set of Authenticator Options.
//
// An optional permutation functon can be provided to modify those Options
// before the Authenticator is created.
func (s *Service) AuthenticatedClient(f func(o *auth.Options)) (*http.Client, error) {
	a, err := s.Authenticator(f)
	if err != nil {
		return nil, err
	}
	return a.Client()
}
