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
	"runtime/pprof"
	"sync/atomic"

	"github.com/luci/luci-go/client/authcli"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/gcloud/gs"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"github.com/luci/luci-go/common/prpc"
	"github.com/luci/luci-go/server/internal/logdog/retryServicesClient"
	"github.com/luci/luci-go/server/internal/logdog/service/config"
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
	// Flags is the set of flags that will be used by the Service.
	Flags flag.FlagSet

	shutdownFunc atomic.Value

	loggingFlags log.Config
	authFlags    authcli.Flags
	configFlags  config.Flags

	coordinatorHost           string
	coordinatorInsecure       bool
	storageCredentialJSONPath string
	cpuProfilePath            string
	heapProfilePath           string

	coord  logdog.ServicesClient
	config *config.Manager
}

// Run performs service-wide initialization and invokes the specified run
// function.
func (s *Service) Run(c context.Context, f func(context.Context) error) {
	c = gologger.Use(c)

	rc := 0
	if err := s.runImpl(c, f); err != nil {
		log.WithError(err).Errorf(c, "Application exiting with error.")
		rc = 1
	}
	os.Exit(rc)
}

func (s *Service) runImpl(c context.Context, f func(context.Context) error) error {
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

	// Configure our signal handler. It will listen for terminating signals and
	// issue a shutdown signal if one is received.
	signalC := make(chan os.Signal)
	go func() {
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
	}()
	signal.Notify(signalC, os.Interrupt)
	defer func() {
		signal.Stop(signalC)
		close(signalC)
	}()

	// Initialize our Client instantiations.
	var err error
	s.coord, err = s.initCoordinatorClient(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to setup Coordinator client.")
		return err
	}

	s.config, err = s.initConfig(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to setup configuration.")
		return err
	}
	defer s.config.Close()

	defer s.SetShutdownFunc(nil)
	return f(c)
}

func (s *Service) addFlags(c context.Context, fs *flag.FlagSet) {
	s.loggingFlags.Level = log.Warning
	s.loggingFlags.AddFlags(fs)

	s.authFlags.Register(fs, auth.Options{
		Context: c,
		Logger:  log.Get(c),
	})
	s.configFlags.AddToFlagSet(fs)

	fs.StringVar(&s.coordinatorHost, "coordinator", "",
		"The Coordinator service's [host][:port].")
	fs.BoolVar(&s.coordinatorInsecure, "coordinator-insecure", false,
		"Connect to Coordinator over HTTP (instead of HTTPS).")
	fs.StringVar(&s.storageCredentialJSONPath, "storage-credential-json-path", "",
		"If supplied, the path of a JSON credential file to load and use for storage operations.")
	fs.StringVar(&s.cpuProfilePath, "cpu-profile-path", "",
		"If supplied, enable CPU profiling and write the profile here.")
	fs.StringVar(&s.heapProfilePath, "heap-profile-path", "",
		"If supplied, enable CPU profiling and write the profile here.")
}

func (s *Service) initCoordinatorClient(c context.Context) (logdog.ServicesClient, error) {
	if s.coordinatorHost == "" {
		log.Errorf(c, "Missing Coordinator URL (-coordinator).")
		return nil, ErrInvalidConfig
	}

	httpClient, err := s.AuthenticatedClient(func(o *auth.Options) {
		o.Scopes = CoordinatorScopes
	})
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create authenticated client.")
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
	sc := logdog.NewServicesPRPCClient(&prpcClient)

	// Wrap the resulting client in a retry harness.
	return retryServicesClient.New(sc, nil), nil
}

func (s *Service) initConfig(c context.Context) (*config.Manager, error) {
	rt, err := s.AuthenticatedTransport(nil)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create config client.")
		return nil, err
	}

	s.configFlags.RoundTripper = rt
	o, err := s.configFlags.CoordinatorOptions(c, s.coord)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to load configuration parameters.")
		return nil, err
	}
	o.KillFunc = s.shutdown

	return config.NewManager(c, *o)
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

// Config returns the cached service configuration.
func (s *Service) Config() *svcconfig.Config {
	return s.config.Config()
}

// Coordinator returns the cached Coordinator client.
func (s *Service) Coordinator() logdog.ServicesClient {
	return s.coord
}

// IntermediateStorage instantiates the configured intermediate Storage
// instance.
func (s *Service) IntermediateStorage(c context.Context) (storage.Storage, error) {
	cfg := s.config.Config()
	if cfg.GetStorage() == nil {
		log.Errorf(c, "Missing storage configuration.")
		return nil, ErrInvalidConfig
	}

	btcfg := cfg.GetStorage().GetBigtable()
	if btcfg == nil {
		log.Errorf(c, "Missing BigTable storage configuration")
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
		log.WithError(err).Errorf(c, "Failed to create BigTable Authenticator.")
		return nil, err
	}

	bt, err := bigtable.New(c, bigtable.Options{
		Project:  btcfg.Project,
		Zone:     btcfg.Zone,
		Cluster:  btcfg.Cluster,
		LogTable: btcfg.LogTableName,
		ClientOptions: []cloud.ClientOption{
			cloud.WithTokenSource(a.TokenSource()),
		},
	})
	if err != nil {
		return nil, err
	}
	return bt, nil
}

// GSClient returns an authenticated Google Storage client instance.
func (s *Service) GSClient(c context.Context) (gs.Client, error) {
	rt, err := s.AuthenticatedTransport(func(o *auth.Options) {
		o.Scopes = gs.ReadWriteScopes
	})
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create authenticated GS transport.")
		return nil, err
	}

	client, err := gs.NewProdClient(c, rt)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Google Storage client.")
		return nil, err
	}
	return client, nil
}

// Authenticator returns an Authenticator instance. The Authenticator is
// configured from a base set of Authenticator Options.
//
// An optional permutation functon can be provided to modify those Options
// before the Authenticator is created.
func (s *Service) Authenticator(f func(o *auth.Options)) (*auth.Authenticator, error) {
	authOpts, err := s.authFlags.Options()
	if err != nil {
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
