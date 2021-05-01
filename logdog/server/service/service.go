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
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	commonAuth "go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	gcps "go.chromium.org/luci/common/gcloud/pubsub"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/sdlogger"
	"go.chromium.org/luci/common/logging/teelogger"
	"go.chromium.org/luci/common/runtime/profiling"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/logdog/server/config"
	serverAuth "go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/gae/impl/cloud"

	"cloud.google.com/go/datastore"
	cl "cloud.google.com/go/logging"
	"cloud.google.com/go/pubsub"

	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

var (
	// ErrInvalidConfig is an error that is returned when the supplied
	// configuration is invalid.
	ErrInvalidConfig = errors.New("invalid configuration")
)

const (
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

	// ServiceID is the cloud project ID specified via -service-id flag.
	//
	// This is synonymous with the cloud "project ID" and the AppEngine "app ID".
	ServiceID string

	loggingFlags log.Config
	authFlags    authcli.Flags
	tsMonFlags   tsmon.Flags
	profiler     profiling.Profiler

	// configStore caches configs in local memory.
	configStore config.Store

	// authCache is a cache of instantiated Authenticator instances, keyed on
	// sorted NULL-delimited scope strings (see authenticatorForScopes).
	authCacheLock sync.RWMutex
	authCache     map[string]*commonAuth.Authenticator
}

// Run performs service-wide initialization and invokes the specified run
// function.
func (s *Service) Run(c context.Context, f func(context.Context) error) {
	// Log to Stdout using fluentd-compatible JSON log lines.
	sink := &sdlogger.Sink{Out: os.Stdout}
	c = teelogger.Use(c, sdlogger.Factory(sink, sdlogger.LogEntry{}, nil))

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

	// Cancel our Context after we're done with the run loop or on a signal.
	c, cancelFunc := context.WithCancel(c)
	defer cancelFunc()

	// Validate the runtime environment.
	if s.ServiceID == "" {
		return errors.New("no service ID was configured (-service-id)")
	}

	// Install our authentication service.
	c = s.withAuthService(c)

	// Install a cloud datastore client.
	dsClient, err := s.initDatastoreClient(c)
	if err != nil {
		return errors.Annotate(err, "failed to initialize datastore client").Err()
	}
	defer dsClient.Close()
	c = (&cloud.Config{DS: dsClient, ProjectID: s.ServiceID}).Use(c, nil)

	// Install a process-wide cache.
	c = caching.WithEmptyProcessCache(c)

	// Add a in-memory config caching to avoid hitting datastore all the time.
	c = config.WithStore(c, &s.configStore)

	// Configure our signal handler. It will listen for terminating signals and
	// issue a shutdown signal if one is received.
	signalC := make(chan os.Signal)
	go func(c context.Context) {
		hasShutdownAlready := false
		for sig := range signalC {
			if !hasShutdownAlready {
				hasShutdownAlready = true

				log.Warningf(log.SetField(c, "signal", sig), "Received close signal. Send again to terminate immediately.")
				cancelFunc()
				continue
			}

			// No shutdown function registered; just exit immediately.
			log.Warningf(c, "Received the second signal, exiting immediately...")
			os.Exit(1)
			panic("never reached")
		}
	}(c)
	signal.Notify(signalC, signals.Interrupts()...)
	defer func() {
		signal.Stop(signalC)
		close(signalC)
	}()

	// Initialize our tsmon library.
	if s.tsMonFlags.Target.TaskServiceName == "" {
		s.tsMonFlags.Target.TaskServiceName = s.ServiceID
	}
	c = tsmon.WithState(c, tsmon.NewState())
	if err := tsmon.InitializeFromFlags(c, &s.tsMonFlags); err != nil {
		return errors.Annotate(err, "failed to initialize monitoring").Err()
	}
	defer tsmon.Shutdown(c)

	// Run main service function.
	return f(c)
}

func (s *Service) addFlags(c context.Context, fs *flag.FlagSet) {
	// Initialize logging flags.
	s.loggingFlags.Level = log.Warning
	s.loggingFlags.AddFlags(fs)

	// Initialize tsmon flags. TaskServiceName will be populated once -service-id
	// is parsed, right before InitializeFromFlags.
	s.tsMonFlags = tsmon.NewFlags()
	s.tsMonFlags.Flush = tsmon.FlushAuto
	s.tsMonFlags.Target.TargetType = target.TaskType
	s.tsMonFlags.Target.TaskJobName = s.Name
	s.tsMonFlags.Register(fs)

	// Initialize auth flags.
	s.authFlags.Register(fs, s.DefaultAuthOptions)

	// Initialize profiling flags.
	s.profiler.AddFlags(fs)

	fs.StringVar(&s.ServiceID, "service-id", s.ServiceID,
		"Specify the service ID that this instance is supporting. This should match the "+
			"App ID of the Coordinator.")
}

func (s *Service) initDatastoreClient(c context.Context) (*datastore.Client, error) {
	ts, err := serverAuth.GetTokenSource(
		c, serverAuth.AsSelf,
		serverAuth.WithScopes(datastore.ScopeDatastore))
	if err != nil {
		return nil, err
	}
	return datastore.NewClient(c, s.ServiceID,
		option.WithUserAgent(s.getUserAgent()),
		option.WithTokenSource(ts))
}

// GSClient returns an authenticated Google Storage client instance.
func (s *Service) GSClient(c context.Context, project string) (gs.Client, error) {
	// TODO(vadimsh): Switch to AsProject + WithProject(project) once
	// we are ready to roll out project scoped service accounts in Logdog.
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

// CLClient returns an authenticated Cloud Logging client instance.
func (s *Service) CLClient(c context.Context, project, dest string, useProjectScope bool) (*cl.Client, error) {
	kind, rpcOpts := serverAuth.AsSelf, []serverAuth.RPCOption{}
	if useProjectScope {
		kind = serverAuth.AsProject
		rpcOpts = append(rpcOpts, serverAuth.WithProject(project))
	}
	cred, err := serverAuth.GetPerRPCCredentials(c, kind, rpcOpts...)
	if err != nil {
		return nil, err
	}

	return cl.NewClient(c, project, option.WithGRPCDialOption(grpc.WithPerRPCCredentials(cred)))
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

func (s *Service) getUserAgent() string { return s.Name + " / " + s.ServiceID }

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
			scopesStr := strings.Join(scopes, " ")
			if err != nil {
				log.Fields{
					"scopes":     scopesStr,
					log.ErrorKey: err,
				}.Errorf(c, "Failed to create authenticator.")
				return nil, err
			}
			tok, err := a.GetAccessToken(minAuthTokenLifetime)
			if err != nil {
				log.Fields{
					"scopes":     scopesStr,
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
