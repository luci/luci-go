// Copyright 2019 The LUCI Authors.
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

// Package server implements an environment for running LUCI servers.
//
// It interprets command line flags and initializes the serving environment with
// the following core services:
//
//   - go.chromium.org/luci/common/logging: logging via Google Cloud Logging.
//   - go.opentelemetry.io/otel/trace: OpenTelemetry tracing with export to
//     Google Cloud Trace.
//   - go.chromium.org/luci/server/tsmon: monitoring metrics via ProdX.
//   - go.chromium.org/luci/server/auth: sending and receiving RPCs
//     authenticated with Google OAuth2 or OpenID tokens. Support for
//     authorization via LUCI groups and LUCI realms.
//   - go.chromium.org/luci/server/caching: in-process caching.
//   - go.chromium.org/luci/server/warmup: allows other server components to
//     register warmup callbacks that run before the server starts handling
//     requests.
//   - go.chromium.org/luci/server/experiments: simple feature flags support.
//   - go.chromium.org/luci/grpc/prpc: pRPC server and RPC Explorer UI.
//   - Error reporting via Google Cloud Error Reporting.
//   - Continuous profiling via Google Cloud Profiler.
//
// Other functionality is optional and provided by modules (objects implementing
// module.Module interface). They should be passed to the server when it starts
// (see the example below). Modules usually expose their configuration via
// command line flags, and provide functionality by injecting state into
// the server's global context.Context or by exposing gRPC endpoints.
//
// Usage example:
//
//	import (
//	  ...
//
//	  "go.chromium.org/luci/server"
//	  "go.chromium.org/luci/server/gaeemulation"
//	  "go.chromium.org/luci/server/module"
//	  "go.chromium.org/luci/server/redisconn"
//	)
//
//	func main() {
//	  modules := []module.Module{
//	    gaeemulation.NewModuleFromFlags(),
//	    redisconn.NewModuleFromFlags(),
//	  }
//	  server.Main(nil, modules, func(srv *server.Server) error {
//	    // Initialize global state, change root context (if necessary).
//	    if err := initializeGlobalStuff(srv.Context); err != nil {
//	      return err
//	    }
//	    srv.Context = injectGlobalStuff(srv.Context)
//
//	    // Install regular HTTP routes.
//	    srv.Routes.GET("/", nil, func(c *router.Context) {
//	      // ...
//	    })
//
//	    // Install gRPC services.
//	    servicepb.RegisterSomeServer(srv, &SomeServer{})
//	    return nil
//	  })
//	}
//
// More examples can be found in the code search: https://source.chromium.org/search?q=%22server.Main%28nil%2C%20modules%2C%22
//
// # Known modules
//
// The following modules (in alphabetical order) are a part of the LUCI
// repository and can be used in any server binary:
//
//   - go.chromium.org/luci/config/server/cfgmodule: provides LUCI Config
//     client, exposes config validation endpoints used by LUCI Config service.
//   - go.chromium.org/luci/server/analytics: generates Google Analytics js
//     snippets for inclusion in a service's web pages.
//   - go.chromium.org/luci/server/bqlog: implements best effort low-overhead
//     structured logging to BigQuery suitable for debug data like access logs.
//   - go.chromium.org/luci/server/cron: allows registering Cloud Scheduler (aka
//     Appengine cron.yaml) handlers, with proper authentication and monitoring
//     metrics.
//   - go.chromium.org/luci/server/encryptedcookies: implements an
//     authentication scheme for HTTP routes based on encrypted cookies and user
//     sessions in some session store.
//   - go.chromium.org/luci/server/dsmapper: provides a way to apply some
//     function to all datastore entities of some particular kind, in parallel,
//     distributing work via Cloud Tasks.
//   - go.chromium.org/luci/server/gaeemulation: implements
//     go.chromium.org/luci/gae Datastore interface via Google Cloud Datastore
//     API. Named so because because it enables migration of GAEv1 apps to GAEv2
//     without touching datastore-related code.
//   - go.chromium.org/luci/server/gerritauth: implements authentication using
//     Gerrit JWTs. Useful if a service is used by a Gerrit frontend plugin.
//   - go.chromium.org/luci/server/limiter: a simple load shedding mechanism
//     that puts a limit on a number of concurrent gRPC requests the server
//     is handling.
//   - go.chromium.org/luci/server/mailer: sending simple emails.
//   - go.chromium.org/luci/server/redisconn: a Redis client. Also enables Redis
//     as a caching backend for go.chromium.org/luci/server/caching and for
//     go.chromium.org/luci/gae/filter/dscache.
//   - go.chromium.org/luci/server/secrets: enables generation and validation of
//     HMAC-tagged tokens via go.chromium.org/luci/server/tokens.
//   - go.chromium.org/luci/server/span: a Cloud Spanner client. Wraps Spanner
//     API a bit to improve interoperability with other modules (in particular
//     the TQ module).
//   - go.chromium.org/luci/server/tq: implements a task queue mechanism on top
//     of Cloud Tasks and Cloud PubSub. Also implements transactional task
//     enqueuing when submitting tasks in a Cloud Datastore or a Cloud Spanner
//     transaction.
//
// Most of them need to be configured via corresponding CLI flags to be useful.
// See implementation of individual modules for details.
//
// An up-to-date list of all known module implementations can be found here:
// https://source.chromium.org/search?q=%22NewModuleFromFlags()%20module.Module%22
//
// # gRPC services
//
// The server implements grpc.ServiceRegistrar interface which means it can be
// used to register gRPC service implementations in. The registered services
// will be exposed via gRPC protocol over the gRPC port (if the gRPC serving
// port is configured in options) and via pRPC protocol over the main HTTP port
// (if the main HTTP serving port is configured in options). The server is also
// pre-configured with a set of gRPC interceptors that collect performance
// metrics, catch panics and authenticate requests. More interceptors can be
// added via RegisterUnaryServerInterceptors.
//
// # Security considerations
//
// The expected deployment environments are Kubernetes, Google App Engine and
// Google Cloud Run. In all cases the server is expected to be behind a load
// balancer or proxy (or a series of load balancers and proxies) that terminate
// TLS and set `X-Forwarded-For` and `X-Forwarded-Proto` headers. In particular
// `X-Forwarded-For` header should look like:
//
//	[<untrusted part>,]<IP that connected to the LB>,<unimportant>[,<more>].
//
// Where `<untrusted part>` may be present if the original request from the
// Internet comes with `X-Forwarded-For` header. The IP specified there is not
// trusted, but the server assumes the load balancer at least sanitizes the
// format of this field.
//
// `<IP that connected to the LB>` is the end-client IP that can be used by the
// server for logs and for IP-allowlist checks.
//
// `<unimportant>` is a "global forwarding rule external IP" for GKE or
// the constant "169.254.1.1" for GAE and Cloud Run. It is unused. See
// https://cloud.google.com/load-balancing/docs/https for more info.
//
// `<more>` may be present if the request was proxied through more layers of
// load balancers while already inside the cluster. The server currently assumes
// this is not happening (i.e. `<more>` is absent, or, in other words, the
// client IP is the second to last in the `X-Forwarded-For` list). If you need
// to recognize more layers of load balancing, please file a feature request to
// add a CLI flag specifying how many layers of load balancers to skip to get to
// the original IP.
package server

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gcemetadata "cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/errorreporting"
	credentials "cloud.google.com/go/iam/credentials/apiv1"
	"cloud.google.com/go/iam/credentials/apiv1/credentialspb"
	"cloud.google.com/go/profiler"
	texporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	gcppropagator "github.com/GoogleCloudPlatform/opentelemetry-operations-go/propagator"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	otelmetricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	oteltracenoop "go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	codepb "google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	clientauth "go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/logging/sdlogger"
	"go.chromium.org/luci/common/system/signals"
	tsmoncommon "go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/hardcoded/chromeinfra" // should be used ONLY in Main()
	"go.chromium.org/luci/web/rpcexplorer"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/authdb/dump"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/experiments"
	"go.chromium.org/luci/server/internal"
	"go.chromium.org/luci/server/internal/gae"
	"go.chromium.org/luci/server/middleware"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/portal"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tsmon"
	"go.chromium.org/luci/server/warmup"
)

const (
	// Path of the health check endpoint.
	healthEndpoint = "/healthz"

	// Log a warning if health check is slower than this.
	healthTimeLogThreshold    = 50 * time.Millisecond
	defaultTsMonFlushInterval = 60 * time.Second
	defaultTsMonFlushTimeout  = 15 * time.Second
)

var (
	versionMetric = metric.NewString(
		"server/version",
		"Version of the running container image (taken from -container-image-id).",
		nil)
)

// cloudRegionFromGAERegion maps GAE region codes (e.g. `s`) to corresponding
// cloud regions (e.g. `us-central1`), which may be defined as regions where GAE
// creates resources associated with the app, such as Task Queues or Flex VMs.
//
// Sadly this mapping is not documented, thus the below map is incomplete. Feel
// free to modify it if you deployed to some new GAE region.
//
// This mapping is unused if `-cloud-region` flag is passed explicitly.
var cloudRegionFromGAERegion = map[string]string{
	"e": "europe-west1",
	"g": "europe-west2",
	"h": "europe-west3",
	"m": "us-west2",
	"p": "us-east1",
	"s": "us-central1",
}

// Context key of *incomingRequest{...}, see httpRoot(...) and grpcRoot(...).
var incomingRequestKey = "go.chromium.org/luci/server.incomingRequest"

// Main initializes the server and runs its serving loop until SIGTERM.
//
// Registers all options in the default flag set and uses `flag.Parse` to parse
// them. If 'opts' is nil, the default options will be used. Only flags are
// allowed in the command line (no positional arguments).
//
// Additionally recognizes GAE_* and K_* env vars as an indicator that the
// server is running in the corresponding serverless runtime. This slightly
// tweaks its behavior to match what these runtimes expects from servers.
//
// On errors, logs them and aborts the process with non-zero exit code.
func Main(opts *Options, mods []module.Module, init func(srv *Server) error) {
	// Prepopulate defaults for flags based on the runtime environment.
	opts, err := OptionsFromEnv(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "When constructing options: %s\n", err)
		os.Exit(3)
	}

	// Register and parse server flags.
	opts.Register(flag.CommandLine)
	flag.Parse()
	if args := flag.Args(); len(args) > 0 {
		fmt.Fprintf(os.Stderr, "got unexpected positional command line arguments: %v\n", args)
		os.Exit(3)
	}

	// Construct the server and run its serving loop.
	srv, err := New(context.Background(), *opts, mods)
	if err != nil {
		srv.Fatal(err)
	}
	if init != nil {
		if err = init(srv); err != nil {
			srv.Fatal(err)
		}
	}
	if err = srv.Serve(); err != nil {
		srv.Fatal(err)
	}
}

// Options are used to configure the server.
//
// Most of them are exposed as command line flags (see Register implementation).
// Some (specific to serverless runtimes) are only settable through code or are
// derived from the environment.
type Options struct {
	Prod       bool              // set when running in production (not on a dev workstation)
	Serverless module.Serverless // set when running in a serverless environment, implies Prod
	Hostname   string            // used for logging and metric fields, default is os.Hostname

	HTTPAddr  string // address to bind the main listening socket to
	GRPCAddr  string // address to bind the gRPC listening socket to
	AdminAddr string // address to bind the admin socket to, ignored on GAE and Cloud Run
	AllowH2C  bool   // if true, allow HTTP/2 Cleartext traffic on non-gRPC HTTP ports

	DefaultRequestTimeout  time.Duration // how long non-internal HTTP handlers are allowed to run, 1 min by default
	InternalRequestTimeout time.Duration // how long "/internal/*" HTTP handlers are allowed to run, 10 min by default
	ShutdownDelay          time.Duration // how long to wait after SIGTERM before shutting down

	ClientAuth       clientauth.Options // base settings for client auth options
	TokenCacheDir    string             // where to cache auth tokens (optional)
	AuthDBProvider   auth.DBProvider    // source of the AuthDB: if set all Auth* options below are ignored
	AuthDBPath       string             // if set, load AuthDB from a file
	AuthServiceHost  string             // hostname of an Auth Service to use
	AuthDBDump       string             // Google Storage path to fetch AuthDB dumps from
	AuthDBSigner     string             // service account that signs AuthDB dumps
	FrontendClientID string             // OAuth2 ClientID for frontend (e.g. user sign in)

	OpenIDRPCAuthEnable   bool                // if true, use OIDC identity tokens for RPC authentication
	OpenIDRPCAuthAudience stringlistflag.Flag // additional allowed OIDC token audiences

	CloudProject string // name of the hosting Google Cloud Project
	CloudRegion  string // name of the hosting Google Cloud region

	TraceSampling string // what portion of traces to upload to Cloud Trace (ignored on GAE and Cloud Run)

	TsMonAccount       string        // service account to flush metrics as
	TsMonServiceName   string        // service name of tsmon target
	TsMonJobName       string        // job name of tsmon target
	TsMonFlushInterval time.Duration // how often to flush metrics
	TsMonFlushTimeout  time.Duration // timeout for flushing

	ProfilingProbability float64 // an [0; 1.0] float with a chance to enable Cloud Profiler in the process
	ProfilingServiceID   string  // service name to associated with profiles in Cloud Profiler

	ContainerImageID string // ID of the container image with this binary, for logs (optional)

	EnableExperiments []string // names of go.chromium.org/luci/server/experiments to enable

	CloudErrorReporting bool // set to true to enable Cloud Error Reporting

	testSeed           int64                   // used to seed rng in tests
	testStdout         sdlogger.LogEntryWriter // mocks stdout in tests
	testStderr         sdlogger.LogEntryWriter // mocks stderr in tests
	testListeners      map[string]net.Listener // addr => net.Listener, for tests
	testDisableTracing bool                    // don't install a tracing backend
}

// OptionsFromEnv prepopulates options based on the runtime environment.
//
// It detects if the process is running on GAE or Cloud Run and adjust options
// accordingly. See FromGAEEnv and FromCloudRunEnv for exact details of how it
// happens.
//
// Either mutates give `opts`, returning it in the end, or (if `opts` is nil)
// create new Options.
func OptionsFromEnv(opts *Options) (*Options, error) {
	if opts == nil {
		opts = &Options{}
	}

	// Populate unset ClientAuth fields with hardcoded defaults.
	authDefaults := chromeinfra.DefaultAuthOptions()
	if opts.ClientAuth.ClientID == "" {
		opts.ClientAuth.ClientID = authDefaults.ClientID
		opts.ClientAuth.ClientSecret = authDefaults.ClientSecret
	}
	if opts.ClientAuth.TokenServerHost == "" {
		opts.ClientAuth.TokenServerHost = authDefaults.TokenServerHost
	}
	if opts.ClientAuth.SecretsDir == "" {
		opts.ClientAuth.SecretsDir = authDefaults.SecretsDir
	}
	if opts.ClientAuth.LoginSessionsHost == "" {
		opts.ClientAuth.LoginSessionsHost = authDefaults.LoginSessionsHost
	}

	// Use CloudOAuthScopes by default when using UserCredentialsMethod auth mode.
	// This is ignored when running in the cloud (the server uses the ambient
	// credentials provided by the environment).
	if len(opts.ClientAuth.Scopes) == 0 {
		opts.ClientAuth.Scopes = auth.CloudOAuthScopes
	}

	// Prepopulate defaults for flags based on the runtime environment.
	opts.FromGAEEnv()
	if err := opts.FromCloudRunEnv(); err != nil {
		return nil, errors.Annotate(err, "failed to probe Cloud Run environment").Err()
	}
	return opts, nil
}

// Register registers the command line flags.
func (o *Options) Register(f *flag.FlagSet) {
	if o.HTTPAddr == "" {
		o.HTTPAddr = "localhost:8800"
	}
	if o.GRPCAddr == "" {
		o.GRPCAddr = "-" // disabled by default
	}
	if o.AdminAddr == "" {
		o.AdminAddr = "localhost:8900"
	}
	if o.DefaultRequestTimeout == 0 {
		o.DefaultRequestTimeout = time.Minute
	}
	if o.InternalRequestTimeout == 0 {
		o.InternalRequestTimeout = 10 * time.Minute
	}
	if o.ShutdownDelay == 0 {
		o.ShutdownDelay = 15 * time.Second
	}
	if o.TsMonFlushInterval == 0 {
		o.TsMonFlushInterval = defaultTsMonFlushInterval
	}
	if o.TsMonFlushTimeout == 0 {
		o.TsMonFlushTimeout = defaultTsMonFlushTimeout
	}
	if o.ProfilingProbability == 0 {
		o.ProfilingProbability = 1.0
	} else if o.ProfilingProbability < 0 {
		o.ProfilingProbability = 0
	}
	f.BoolVar(&o.Prod, "prod", o.Prod, "Switch the server into production mode")
	f.StringVar(&o.HTTPAddr, "http-addr", o.HTTPAddr, "Address to bind the main listening socket to or '-' to disable")
	f.StringVar(&o.GRPCAddr, "grpc-addr", o.GRPCAddr, "Address to bind the gRPC listening socket to or '-' to disable")
	f.StringVar(&o.AdminAddr, "admin-addr", o.AdminAddr, "Address to bind the admin socket to or '-' to disable")
	f.BoolVar(&o.AllowH2C, "allow-h2c", o.AllowH2C, "If set, allow HTTP/2 Cleartext traffic on non-gRPC HTTP ports (in addition to HTTP/1 traffic). The gRPC port always allows it, it is essential for gRPC")
	f.DurationVar(&o.DefaultRequestTimeout, "default-request-timeout", o.DefaultRequestTimeout, "How long incoming HTTP requests are allowed to run before being canceled (or 0 for infinity)")
	f.DurationVar(&o.InternalRequestTimeout, "internal-request-timeout", o.InternalRequestTimeout, "How long incoming /internal/* HTTP requests are allowed to run before being canceled (or 0 for infinity)")
	f.DurationVar(&o.ShutdownDelay, "shutdown-delay", o.ShutdownDelay, "How long to wait after SIGTERM before shutting down")
	f.StringVar(
		&o.ClientAuth.ServiceAccountJSONPath,
		"service-account-json",
		o.ClientAuth.ServiceAccountJSONPath,
		"Path to a JSON file with service account private key",
	)
	f.StringVar(
		&o.ClientAuth.ActAsServiceAccount,
		"act-as",
		o.ClientAuth.ActAsServiceAccount,
		"Act as this service account",
	)
	f.StringVar(
		&o.TokenCacheDir,
		"token-cache-dir",
		o.TokenCacheDir,
		"Where to cache auth tokens (optional)",
	)
	f.StringVar(
		&o.AuthDBPath,
		"auth-db-path",
		o.AuthDBPath,
		"If set, load AuthDB text proto from this file (incompatible with -auth-service-host)",
	)
	f.StringVar(
		&o.AuthServiceHost,
		"auth-service-host",
		o.AuthServiceHost,
		"Hostname of an Auth Service to use (incompatible with -auth-db-path)",
	)
	f.StringVar(
		&o.AuthDBDump,
		"auth-db-dump",
		o.AuthDBDump,
		"Google Storage path to fetch AuthDB dumps from. Default is gs://<auth-service-host>/auth-db",
	)
	f.StringVar(
		&o.AuthDBSigner,
		"auth-db-signer",
		o.AuthDBSigner,
		"Service account that signs AuthDB dumps. Default is derived from -auth-service-host if it is *.appspot.com",
	)
	f.StringVar(
		&o.FrontendClientID,
		"frontend-client-id",
		o.FrontendClientID,
		"OAuth2 clientID for use in frontend, e.g. for user sign in (optional)",
	)
	f.BoolVar(
		&o.OpenIDRPCAuthEnable,
		"open-id-rpc-auth-enable",
		o.OpenIDRPCAuthEnable,
		"If set accept OpenID Connect ID tokens as per-RPC credentials",
	)
	f.Var(
		&o.OpenIDRPCAuthAudience,
		"open-id-rpc-auth-audience",
		"Additional accepted value of `aud` claim in OpenID tokens, can be repeated",
	)
	f.StringVar(
		&o.CloudProject,
		"cloud-project",
		o.CloudProject,
		"Name of hosting Google Cloud Project (optional)",
	)
	f.StringVar(
		&o.CloudRegion,
		"cloud-region",
		o.CloudRegion,
		"Name of hosting Google Cloud region, e.g. 'us-central1' (optional)",
	)
	f.StringVar(
		&o.TraceSampling,
		"trace-sampling",
		o.TraceSampling,
		"What portion of traces to upload to Cloud Trace. Either a percent (i.e. '0.1%') or a QPS (i.e. '1qps'). Ignored on GAE and Cloud Run. Default is 0.1qps.",
	)
	f.StringVar(
		&o.TsMonAccount,
		"ts-mon-account",
		o.TsMonAccount,
		"Collect and flush tsmon metrics using this account for auth (disables tsmon if not set)",
	)
	f.StringVar(
		&o.TsMonServiceName,
		"ts-mon-service-name",
		o.TsMonServiceName,
		"Service name of tsmon target (disables tsmon if not set)",
	)
	f.StringVar(
		&o.TsMonJobName,
		"ts-mon-job-name",
		o.TsMonJobName,
		"Job name of tsmon target (disables tsmon if not set)",
	)
	f.DurationVar(
		&o.TsMonFlushInterval,
		"ts-mon-flush-interval",
		o.TsMonFlushInterval,
		fmt.Sprintf("How often to flush tsmon metrics. Default to %s if < 1s or unset", o.TsMonFlushInterval),
	)
	f.DurationVar(
		&o.TsMonFlushTimeout,
		"ts-mon-flush-timeout",
		o.TsMonFlushTimeout,
		fmt.Sprintf("Timeout for tsmon flush. Default to %s if < 1s or unset. Must be shorter than --ts-mon-flush-interval.", o.TsMonFlushTimeout),
	)
	f.Float64Var(
		&o.ProfilingProbability,
		"profiling-probability",
		o.ProfilingProbability,
		fmt.Sprintf("A float [0; 1.0] with probability to enable Cloud Profiler for the current process. Default is %f.", o.ProfilingProbability),
	)
	f.StringVar(
		&o.ProfilingServiceID,
		"profiling-service-id",
		o.ProfilingServiceID,
		"Service name to associated with profiles in Cloud Profiler. Defaults to the value of -ts-mon-job-name.",
	)
	f.StringVar(
		&o.ContainerImageID,
		"container-image-id",
		o.ContainerImageID,
		"ID of the container image with this binary, for logs (optional)",
	)
	f.BoolVar(
		&o.CloudErrorReporting,
		"cloud-error-reporting",
		o.CloudErrorReporting,
		"Enable Cloud Error Reporting",
	)

	// See go.chromium.org/luci/server/experiments.
	f.Var(luciflag.StringSlice(&o.EnableExperiments), "enable-experiment",
		`A name of the experiment to enable. May be repeated.`)
}

// FromGAEEnv uses the GAE_* env vars to configure the server for the GAE
// environment.
//
// Does nothing if GAE_VERSION is not set.
//
// Equivalent to passing the following flags:
//
//	-prod
//	-http-addr 0.0.0.0:${PORT}
//	-admin-addr -
//	-shutdown-delay 1s
//	-cloud-project ${GOOGLE_CLOUD_PROJECT}
//	-cloud-region <derived from the region code in GAE_APPLICATION>
//	-service-account-json :gce
//	-ts-mon-service-name ${GOOGLE_CLOUD_PROJECT}
//	-ts-mon-job-name ${GAE_SERVICE}
//
// Additionally the hostname and -container-image-id (used in metric and trace
// fields) are derived from available GAE_* env vars to be semantically similar
// to what they represent in the GKE environment.
//
// Note that a mapping between a region code in GAE_APPLICATION and
// the corresponding cloud region is not documented anywhere, so if you see
// warnings when your app starts up either update the code to recognize your
// region code or pass '-cloud-region' argument explicitly in app.yaml.
//
// See https://cloud.google.com/appengine/docs/standard/go/runtime.
func (o *Options) FromGAEEnv() {
	if os.Getenv("GAE_VERSION") == "" {
		return
	}
	o.Serverless = module.GAE
	o.Prod = true
	o.Hostname = uniqueServerlessHostname(
		os.Getenv("GAE_SERVICE"),
		os.Getenv("GAE_DEPLOYMENT_ID"),
		os.Getenv("GAE_INSTANCE"),
	)
	o.HTTPAddr = fmt.Sprintf("0.0.0.0:%s", os.Getenv("PORT"))
	o.GRPCAddr = "-"
	o.AdminAddr = "-"
	o.ShutdownDelay = time.Second
	o.CloudProject = os.Getenv("GOOGLE_CLOUD_PROJECT")
	o.ClientAuth.ServiceAccountJSONPath = clientauth.GCEServiceAccount
	o.ClientAuth.GCESupportsArbitraryScopes = true
	o.TsMonServiceName = os.Getenv("GOOGLE_CLOUD_PROJECT")
	o.TsMonJobName = os.Getenv("GAE_SERVICE")
	o.ContainerImageID = fmt.Sprintf("appengine/%s/%s:%s",
		os.Getenv("GOOGLE_CLOUD_PROJECT"),
		os.Getenv("GAE_SERVICE"),
		os.Getenv("GAE_VERSION"),
	)
	// Note: GAE_APPLICATION is missing on Flex.
	if appID := os.Getenv("GAE_APPLICATION"); appID != "" && o.CloudRegion == "" {
		o.CloudRegion = cloudRegionFromGAERegion[strings.Split(appID, "~")[0]]
	}
}

// FromCloudRunEnv recognized K_SERVICE environment variable and configures
// some options based on what it discovers in the environment.
//
// Does nothing if K_SERVICE is not set.
//
// Equivalent to passing the following flags:
//
//	-prod
//	-http-addr -
//	-grpc-addr -
//	-admin-addr -
//	-allow-h2c
//	-shutdown-delay 1s
//	-cloud-project <cloud project Cloud Run container is running in>
//	-cloud-region <cloud region Cloud Run container is running in>
//	-service-account-json :gce
//	-open-id-rpc-auth-enable
//	-ts-mon-service-name <cloud project Cloud Run container is running in>
//	-ts-mon-job-name ${K_SERVICE}
//
// Flags passed via the actual command line in the Cloud Run manifest override
// these prefilled defaults. In particular pass either `-http-addr` or
// `-grpc-addr` (or both) to enable corresponding ports.
//
// Additionally the hostname (used in metric and trace fields) is derived from
// environment to be semantically similar to what it looks like in the GKE
// environment.
func (o *Options) FromCloudRunEnv() error {
	if os.Getenv("K_SERVICE") == "" {
		return nil
	}

	// See https://cloud.google.com/run/docs/container-contract.
	project, err := gcemetadata.Get("project/project-id")
	if err != nil {
		return errors.Annotate(err, "failed to get the project ID").Err()
	}
	region, err := gcemetadata.Get("instance/region")
	if err != nil {
		return errors.Annotate(err, "failed to get the cloud region").Err()
	}
	// Region format returned by Cloud Run is `projects/PROJECT-NUMBER/regions/REGION`
	parts := strings.Split(region, "/")
	region = parts[len(parts)-1]
	instance, err := gcemetadata.Get("instance/id")
	if err != nil {
		return errors.Annotate(err, "failed to get the instance ID").Err()
	}

	o.Serverless = module.CloudRun
	o.Prod = true
	o.Hostname = uniqueServerlessHostname(os.Getenv("K_REVISION"), instance)
	o.HTTPAddr = "-"
	o.GRPCAddr = "-"
	o.AdminAddr = "-"
	o.AllowH2C = true // to allow using HTTP2 end-to-end with `--use-http2` deployment flag
	o.ShutdownDelay = time.Second
	o.CloudProject = project
	o.CloudRegion = region
	o.ClientAuth.ServiceAccountJSONPath = clientauth.GCEServiceAccount
	o.ClientAuth.GCESupportsArbitraryScopes = true
	o.OpenIDRPCAuthEnable = true
	o.TsMonServiceName = project
	o.TsMonJobName = os.Getenv("K_SERVICE")

	return nil
}

// uniqueServerlessHostname generates a hostname to use when running in a GCP
// serverless environment.
//
// Unlike GKE or GCE environments, serverless containers do not have a proper
// unique hostname set, but we still need to identify them uniquely in logs
// and monitoring metrics. They do have a giant hex instance ID string, but it
// is not informative on its own and cumbersome to use.
//
// This functions produces a reasonably readable and unique string that looks
// like `parts[0]-parts[1]-...-hash(parts[last])`. It assumes the last string
// in `parts` is the giant instance ID.
func uniqueServerlessHostname(parts ...string) string {
	id := sha256.Sum256([]byte(parts[len(parts)-1]))
	parts[len(parts)-1] = hex.EncodeToString(id[:])[:16]
	return strings.Join(parts, "-")
}

// ImageVersion extracts image tag or digest from ContainerImageID.
//
// This is eventually reported as a value of 'server/version' metric.
//
// On GAE it would return the service version name based on GAE_VERSION env var,
// since ContainerImageID is artificially constructed to look like
// "appengine/${CLOUD_PROJECT}/${GAE_SERVICE}:${GAE_VERSION}".
//
// On Cloud Run it is responsibility of the deployment layer to correctly
// populate -container-image-id command line flag.
//
// Returns "unknown" if ContainerImageID is empty or malformed.
func (o *Options) ImageVersion() string {
	// Recognize "<path>@sha256:<digest>" and "<path>:<tag>".
	idx := strings.LastIndex(o.ContainerImageID, "@")
	if idx == -1 {
		idx = strings.LastIndex(o.ContainerImageID, ":")
	}
	if idx == -1 {
		return "unknown"
	}
	return o.ContainerImageID[idx+1:]
}

// ImageName extracts image name from ContainerImageID.
//
// This is the part of ContainerImageID before ':' or '@'.
func (o *Options) ImageName() string {
	// Recognize "<path>@sha256:<digest>" and "<path>:<tag>".
	idx := strings.LastIndex(o.ContainerImageID, "@")
	if idx == -1 {
		idx = strings.LastIndex(o.ContainerImageID, ":")
	}
	if idx == -1 {
		return "unknown"
	}
	return o.ContainerImageID[:idx]
}

// userAgent derives a user-agent like string identifying the server.
func (o *Options) userAgent() string {
	return fmt.Sprintf("LUCI-Server (service: %s; job: %s; ver: %s);", o.TsMonServiceName, o.TsMonJobName, o.ImageVersion())
}

// shouldEnableTracing is true if options indicate we should enable tracing.
func (o *Options) shouldEnableTracing() bool {
	switch {
	case o.CloudProject == "":
		return false // nowhere to upload traces to
	case !o.Prod && o.TraceSampling == "":
		return false // in dev mode don't upload samples by default
	default:
		return !o.testDisableTracing
	}
}

// hostOptions constructs HostOptions for module.Initialize(...).
func (o *Options) hostOptions() module.HostOptions {
	return module.HostOptions{
		Prod:         o.Prod,
		Serverless:   o.Serverless,
		CloudProject: o.CloudProject,
		CloudRegion:  o.CloudRegion,
	}
}

// Server is responsible for initializing and launching the serving environment.
//
// Generally assumed to be a singleton: do not launch multiple Server instances
// within the same process, use AddPort instead if you want to expose multiple
// HTTP ports with different routers.
//
// Server can serve plain HTTP endpoints, routing them trough a router.Router,
// and gRPC APIs (exposing them over gRPC and pRPC protocols). Use an instance
// of Server as a grpc.ServiceRegistrar when registering gRPC services. Services
// registered that way will be available via gRPC protocol over the gRPC port
// and via pRPC protocol over the main HTTP port. Interceptors can be added via
// RegisterUnaryServerInterceptors. RPC authentication can be configured via
// SetRPCAuthMethods.
//
// pRPC protocol is served on the same port as the main HTTP router, making it
// possible to expose just a single HTTP port for everything (which is a
// requirement on Appengine).
//
// Native gRPC protocol is always served though a dedicated gRPC h2c port since
// the gRPC library has its own HTTP/2 server implementation not compatible
// with net/http package used everywhere else. There's an assortments of hacks
// to workaround this, but many ultimately depend on experimental and slow
// grpc.Server.ServeHTTP method. See https://github.com/grpc/grpc-go/issues/586
// and https://github.com/grpc/grpc-go/issues/4620. Another often recommended
// workaround is https://github.com/soheilhy/cmux, which decides if a new
// connection is a gRPC one or a regular HTTP/2 one. It doesn't work when the
// server is running behind a load balancer that understand HTTP/2, since it
// just opens a **single** backend connection and sends both gRPC and regular
// HTTP/2 requests over it. This happens on Cloud Run, for example. See e.g.
// https://ahmet.im/blog/grpc-http-mux-go/.
//
// If you want to serve HTTP and gRPC over the same public port, configure your
// HTTP load balancer (e.g. https://cloud.google.com/load-balancing/docs/https)
// to route requests into appropriate containers and ports. Another alternative
// is to put an HTTP/2 proxy (e.g. Envoy) right into the pod with the server
// process and route traffic "locally" there. This option would also allow to
// add local grpc-web proxy into the mix if necessary.
//
// The server doesn't do TLS termination (even for gRPC traffic). It must be
// sitting behind a load balancer or a proxy that terminates TLS and sends clear
// text (HTTP/1 or HTTP/2 for gRPC) requests to corresponding ports, injecting
// `X-Forwarded-*` headers. See "Security considerations" section above for more
// details.
type Server struct {
	// Context is the root context used by all requests and background activities.
	//
	// Can be replaced (by a derived context) before Serve call, for example to
	// inject values accessible to all request handlers.
	Context context.Context

	// Routes is a router for requests hitting HTTPAddr port.
	//
	// This router is used for all requests whose Host header does not match any
	// specially registered per-host routers (see VirtualHost). Normally, there
	// are no such per-host routers, so usually Routes is used for all requests.
	//
	// This router is also accessible to the server modules and they can install
	// routes into it.
	//
	// Should be populated before Serve call.
	Routes *router.Router

	// CookieAuth is an authentication method implemented via cookies.
	//
	// It is initialized only if the server has a module implementing such scheme
	// (e.g. "go.chromium.org/luci/server/encryptedcookies").
	CookieAuth auth.Method

	// Options is a copy of options passed to New.
	Options Options

	startTime   time.Time    // for calculating uptime for /healthz
	lastReqTime atomic.Value // time.Time when the last request started

	stdout       sdlogger.LogEntryWriter                   // for logging to stdout, nil in dev mode
	stderr       sdlogger.LogEntryWriter                   // for logging to stderr, nil in dev mode
	errRptClient *errorreporting.Client                    // for reporting to the cloud Error Reporting
	logRequestCB func(context.Context, *sdlogger.LogEntry) // if non-nil, need to emit request log entries via it

	mainPort *Port        // pre-registered main HTTP port, see initMainPort
	grpcPort *grpcPort    // non-nil when exposing a gRPC port
	prpc     *prpc.Server // pRPC server implementation exposed on the main port

	mu      sync.Mutex    // protects fields below
	ports   []servingPort // all non-dummy ports (each one bound to a TCP socket)
	started bool          // true inside and after Serve
	stopped bool          // true inside and after Shutdown
	ready   chan struct{} // closed right before starting the serving loop
	done    chan struct{} // closed after Shutdown returns

	// gRPC/pRPC configuration.
	unaryInterceptors  []grpc.UnaryServerInterceptor
	streamInterceptors []grpc.StreamServerInterceptor
	rpcAuthMethods     []auth.Method

	rndM sync.Mutex // protects rnd
	rnd  *rand.Rand // used to generate trace and operation IDs

	bgrDone chan struct{}  // closed to stop background activities
	bgrWg   sync.WaitGroup // waits for RunInBackground goroutines to stop

	warmupM sync.Mutex // protects 'warmup' and the actual warmup critical section
	warmup  []func(context.Context)

	cleanupM sync.Mutex // protects 'cleanup' and the actual cleanup critical section
	cleanup  []func(context.Context)

	tsmon      *tsmon.State                  // manages flushing of tsmon metrics
	propagator propagation.TextMapPropagator // knows how to propagate trace headers

	cloudTS     oauth2.TokenSource // source of cloud-scoped tokens for Cloud APIs
	signer      *signerImpl        // the signer used by the auth system
	actorTokens *actorTokensImpl   // for impersonating service accounts
	authDB      atomic.Value       // if not using AuthDBProvider, the last known good authdb.DB instance

	runningAs string // email of an account the server runs as
}

// servingPort represents either an HTTP or gRPC serving port.
type servingPort interface {
	nameForLog() string
	serve(baseCtx func() context.Context) error
	shutdown(ctx context.Context)
}

// moduleHostImpl implements module.Host via server.Server.
//
// Just a tiny wrapper to make sure modules consume only curated limited set of
// the server API and do not retain the pointer to the server.
type moduleHostImpl struct {
	srv        *Server
	mod        module.Module
	invalid    bool
	cookieAuth auth.Method
}

func (h *moduleHostImpl) panicIfInvalid() {
	if h.invalid {
		panic("module.Host must not be used outside of Initialize")
	}
}

func (h *moduleHostImpl) HTTPAddr() net.Addr {
	h.panicIfInvalid()
	if h.srv.mainPort.listener != nil {
		return h.srv.mainPort.listener.Addr()
	}
	return nil
}

func (h *moduleHostImpl) GRPCAddr() net.Addr {
	h.panicIfInvalid()
	if h.srv.grpcPort != nil {
		return h.srv.grpcPort.listener.Addr()
	}
	return nil
}

func (h *moduleHostImpl) Routes() *router.Router {
	h.panicIfInvalid()
	return h.srv.Routes
}

func (h *moduleHostImpl) RunInBackground(activity string, f func(context.Context)) {
	h.panicIfInvalid()
	h.srv.RunInBackground(activity, f)
}

func (h *moduleHostImpl) RegisterWarmup(cb func(context.Context)) {
	h.panicIfInvalid()
	h.srv.RegisterWarmup(cb)
}

func (h *moduleHostImpl) RegisterCleanup(cb func(context.Context)) {
	h.panicIfInvalid()
	h.srv.RegisterCleanup(cb)
}

func (h *moduleHostImpl) RegisterService(desc *grpc.ServiceDesc, impl any) {
	h.panicIfInvalid()
	h.srv.RegisterService(desc, impl)
}

func (h *moduleHostImpl) RegisterUnaryServerInterceptors(intr ...grpc.UnaryServerInterceptor) {
	h.panicIfInvalid()
	h.srv.RegisterUnaryServerInterceptors(intr...)
}

func (h *moduleHostImpl) RegisterStreamServerInterceptors(intr ...grpc.StreamServerInterceptor) {
	h.panicIfInvalid()
	h.srv.RegisterStreamServerInterceptors(intr...)
}

func (h *moduleHostImpl) RegisterCookieAuth(method auth.Method) {
	h.panicIfInvalid()
	h.cookieAuth = method
}

// New constructs a new server instance.
//
// It hosts one or more HTTP servers and starts and stops them in unison. It is
// also responsible for preparing contexts for incoming requests.
//
// The given context will become the root context of the server and will be
// inherited by all handlers.
//
// On errors returns partially initialized server (always non-nil). At least
// its logging will be configured and can be used to report the error. Trying
// to use such partially initialized server for anything else is undefined
// behavior.
func New(ctx context.Context, opts Options, mods []module.Module) (srv *Server, err error) {
	seed := opts.testSeed
	if seed == 0 {
		if err := binary.Read(cryptorand.Reader, binary.BigEndian, &seed); err != nil {
			panic(err)
		}
	}

	srv = &Server{
		Context:   ctx,
		Options:   opts,
		startTime: clock.Now(ctx).UTC(),
		ready:     make(chan struct{}),
		done:      make(chan struct{}),
		rnd:       rand.New(rand.NewSource(seed)),
		bgrDone:   make(chan struct{}),
	}

	// Cleanup what we can on failures.
	defer func() {
		if err != nil {
			srv.runCleanup()
		}
	}()

	// Logging is needed to report any errors during the early initialization.
	srv.initLogging()

	logging.Infof(srv.Context, "Server starting...")
	if srv.Options.ContainerImageID != "" {
		logging.Infof(srv.Context, "Container image is %s", srv.Options.ContainerImageID)
	}

	// Need the hostname (e.g. pod name on k8s) for logs and metrics.
	if srv.Options.Hostname == "" {
		srv.Options.Hostname, err = os.Hostname()
		if err != nil {
			return srv, errors.Annotate(err, "failed to get own hostname").Err()
		}
	}

	switch srv.Options.Serverless {
	case module.GAE:
		logging.Infof(srv.Context, "Running on %s", srv.Options.Hostname)
		logging.Infof(srv.Context, "Instance is %q", os.Getenv("GAE_INSTANCE"))
		if srv.Options.CloudRegion == "" {
			if appID := os.Getenv("GAE_APPLICATION"); appID != "" {
				logging.Warningf(srv.Context, "Could not figure out the primary Cloud region based "+
					"on the region code in GAE_APPLICATION %q, consider passing the region name "+
					"via -cloud-region flag explicitly", appID)
			}
		} else {
			logging.Infof(srv.Context, "Cloud region is %s", srv.Options.CloudRegion)
		}
		// Initialize default tickets for background activities. These tickets are
		// overridden in per-request contexts with request-specific tickets.
		srv.Context = gae.WithTickets(srv.Context, gae.DefaultTickets())
	case module.CloudRun:
		logging.Infof(srv.Context, "Running on %s", srv.Options.Hostname)
		logging.Infof(srv.Context, "Revision is %q", os.Getenv("K_REVISION"))
	default:
		// On k8s log pod IPs too, this is useful when debugging k8s routing.
		logging.Infof(srv.Context, "Running on %s (%s)", srv.Options.Hostname, networkAddrsForLog())
	}

	// Log enabled experiments, warn if some of them are unknown now.
	var exps []experiments.ID
	for _, name := range opts.EnableExperiments {
		if exp, ok := experiments.GetByName(name); ok {
			logging.Infof(ctx, "Enabling experiment %q", name)
			exps = append(exps, exp)
		} else {
			logging.Warningf(ctx, "Skipping unknown experiment %q", name)
		}
	}
	srv.Context = experiments.Enable(srv.Context, exps...)

	// Configure base server subsystems by injecting them into the root context
	// inherited later by all requests.
	srv.Context = caching.WithProcessCacheData(srv.Context, caching.NewProcessCacheData())
	if err := srv.initAuthStart(); err != nil {
		return srv, errors.Annotate(err, "failed to initialize auth").Err()
	}
	if err := srv.initTSMon(); err != nil {
		return srv, errors.Annotate(err, "failed to initialize tsmon").Err()
	}
	if err := srv.initAuthFinish(); err != nil {
		return srv, errors.Annotate(err, "failed to finish auth initialization").Err()
	}
	if err := srv.initOpenTelemetry(); err != nil {
		return srv, errors.Annotate(err, "failed to initialize tracing").Err()
	}
	if err := srv.initErrorReporting(); err != nil {
		return srv, errors.Annotate(err, "failed to initialize error reporting").Err()
	}
	if err := srv.initProfiling(); err != nil {
		return srv, errors.Annotate(err, "failed to initialize profiling").Err()
	}
	if err := srv.initMainPort(); err != nil {
		return srv, errors.Annotate(err, "failed to initialize the main port").Err()
	}
	if err := srv.initGrpcPort(); err != nil {
		return srv, errors.Annotate(err, "failed to initialize the gRPC port").Err()
	}
	if err := srv.initAdminPort(); err != nil {
		return srv, errors.Annotate(err, "failed to initialize the admin port").Err()
	}
	if err := srv.initWarmup(); err != nil {
		return srv, errors.Annotate(err, "failed to initialize warmup callbacks").Err()
	}

	// Sort modules by their initialization order based on declared dependencies,
	// discover unfulfilled required dependencies.
	sorted, err := resolveDependencies(mods)
	if err != nil {
		return srv, err
	}

	// Initialize all modules in their topological order.
	impls := make([]*moduleHostImpl, len(sorted))
	for i, mod := range sorted {
		impls[i] = &moduleHostImpl{srv: srv, mod: mod}
		switch ctx, err := mod.Initialize(srv.Context, impls[i], srv.Options.hostOptions()); {
		case err != nil:
			return srv, errors.Annotate(err, "failed to initialize module %q", mod.Name()).Err()
		case ctx != nil:
			srv.Context = ctx
		}
		impls[i].invalid = true // make sure the module does not retain it
	}

	// Ensure there's only one CookieAuth method registered.
	var cookieAuthMod module.Module
	for _, impl := range impls {
		if impl.cookieAuth != nil {
			if cookieAuthMod != nil {
				return srv, errors.Annotate(err,
					"conflict between %q and %q: both register a cookie auth scheme - pick one",
					cookieAuthMod.Name(), impl.mod.Name(),
				).Err()
			}
			cookieAuthMod = impl.mod
			srv.CookieAuth = impl.cookieAuth
		}
	}

	// Install the RPC Explorer, using the registered auth method if it is
	// compatible.
	rpcExpAuth, _ := srv.CookieAuth.(rpcexplorer.AuthMethod)
	rpcexplorer.Install(srv.Routes, rpcExpAuth)

	return srv, nil
}

// AddPort prepares and binds an additional serving HTTP port.
//
// Can be used to open more listening HTTP ports (in addition to opts.HTTPAddr
// and opts.AdminAddr). The returned Port object can be used to populate the
// router that serves requests hitting the added port.
//
// If opts.ListenAddr is '-', a dummy port will be added: it is a valid *Port
// object, but it is not actually exposed as a listening TCP socket. This is
// useful to disable listening ports without changing any code.
//
// Must be called before Serve (panics otherwise).
func (s *Server) AddPort(opts PortOptions) (*Port, error) {
	port := &Port{
		Routes:   s.newRouter(opts),
		parent:   s,
		opts:     opts,
		allowH2C: s.Options.AllowH2C,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		s.Fatal(errors.Reason("the server has already been started").Err())
	}

	if opts.ListenAddr != "-" {
		var err error
		if port.listener, err = s.createListener(opts.ListenAddr); err != nil {
			return nil, errors.Annotate(err, "failed to bind the listening port for %q at %q", opts.Name, opts.ListenAddr).Err()
		}
		// Add to the list of ports that actually have sockets listening.
		s.ports = append(s.ports, port)
	}

	return port, nil
}

// VirtualHost returns a router (registering it if necessary) used for requests
// that hit the main port (opts.HTTPAddr) and have the given Host header.
//
// Should be used in rare cases when the server is exposed through multiple
// domain names and requests should be routed differently based on what domain
// was used. If your server is serving only one domain name, or you don't care
// what domain name is used to access it, do not use VirtualHost.
//
// Note that requests that match some registered virtual host router won't
// reach the default router (server.Routes), even if the virtual host router
// doesn't have a route for them. Such requests finish with HTTP 404.
//
// Also the router created by VirtualHost is initially completely empty: the
// server and its modules don't install anything into it (there's intentionally
// no mechanism to do this). For that reason VirtualHost should never by used to
// register a router for the "main" domain name: it will make the default
// server.Routes (and all handlers installed there by server modules) useless,
// probably breaking the server. Put routes for the main server functionality
// directly into server.Routes instead, using VirtualHost only for routes that
// critically depend on Host header.
//
// Must be called before Serve (panics otherwise).
func (s *Server) VirtualHost(host string) *router.Router {
	return s.mainPort.VirtualHost(host)
}

// createListener creates a TCP listener on the given address.
func (s *Server) createListener(addr string) (net.Listener, error) {
	// If not running tests, bind the socket as usual.
	if s.Options.testListeners == nil {
		return net.Listen("tcp", addr)
	}
	// In test mode the listener MUST be prepared already.
	l := s.Options.testListeners[addr]
	if l == nil {
		return nil, errors.Reason("test listener is not set").Err()
	}
	return l, nil
}

// newRouter creates a Router with the default middleware chain and routes.
func (s *Server) newRouter(opts PortOptions) *router.Router {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		s.Fatal(errors.Reason("the server has already been started").Err())
	}

	// This is a chain of router.Middleware. It is preceded by a chain of raw
	// net/http middlewares (see wrapHTTPHandler):
	//   * s.httpRoot: initializes *incomingRequest in the context.
	//   * otelhttp.NewHandler: opens a tracing span.
	//   * s.httpDispatch: finishes the context initialization.
	mw := router.NewMiddlewareChain(
		middleware.WithPanicCatcher, // transforms panics into HTTP 500
	)
	if s.tsmon != nil && !opts.DisableMetrics {
		mw = mw.Extend(s.tsmon.Middleware) // collect HTTP requests metrics
	}

	// Setup middleware chain used by ALL requests.
	r := router.New()
	r.Use(mw)

	// Mandatory health check/readiness probe endpoint.
	r.GET(healthEndpoint, nil, func(c *router.Context) {
		c.Writer.Write([]byte(s.healthResponse(c.Request.Context())))
	})

	// Add NotFound handler wrapped in our middlewares so that unrecognized
	// requests are at least logged. If we don't do that they'll be handled
	// completely silently and this is very confusing when debugging 404s.
	r.NotFound(nil, func(c *router.Context) {
		http.NotFound(c.Writer, c.Request)
	})

	return r
}

// RunInBackground launches the given callback in a separate goroutine right
// before starting the serving loop.
//
// If the server is already running, launches it right away. If the server
// fails to start, the goroutines will never be launched.
//
// Should be used for background asynchronous activities like reloading configs.
//
// All logs lines emitted by the callback are annotated with "activity" field
// which can be arbitrary, but by convention has format "<namespace>.<name>",
// where "luci" namespace is reserved for internal activities.
//
// The context passed to the callback is canceled when the server is shutting
// down. It is expected the goroutine will exit soon after the context is
// canceled.
func (s *Server) RunInBackground(activity string, f func(context.Context)) {
	s.bgrWg.Add(1)
	go func() {
		defer s.bgrWg.Done()

		select {
		case <-s.ready:
			// Construct the context after the server is fully initialized. Cancel it
			// as soon as bgrDone is signaled.
			ctx, cancel := context.WithCancel(s.Context)
			if activity != "" {
				ctx = logging.SetField(ctx, "activity", activity)
			}
			defer cancel()
			go func() {
				select {
				case <-s.bgrDone:
					cancel()
				case <-ctx.Done():
				}
			}()
			f(ctx)

		case <-s.bgrDone:
			// the server is closed, no need to run f() anymore
		}
	}()
}

// RegisterService is part of grpc.ServiceRegistrar interface.
//
// The registered service will be exposed through both gRPC and pRPC protocols
// on corresponding ports. See Server doc.
//
// Must be called before Serve (panics otherwise).
func (s *Server) RegisterService(desc *grpc.ServiceDesc, impl any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		s.Fatal(errors.Reason("the server has already been started").Err())
	}
	s.prpc.RegisterService(desc, impl)
	if s.grpcPort != nil {
		s.grpcPort.registerService(desc, impl)
	}
}

// RegisterUnaryServerInterceptors registers grpc.UnaryServerInterceptor's
// applied to all unary RPCs that hit the server.
//
// Interceptors are chained in order they are registered, i.e. the first
// registered interceptor becomes the outermost. The initial chain already
// contains some base interceptors (e.g. for monitoring) and all interceptors
// registered by server modules. RegisterUnaryServerInterceptors extends this
// chain. Subsequent calls to RegisterUnaryServerInterceptors adds more
// interceptors into the chain.
//
// Must be called before Serve (panics otherwise).
func (s *Server) RegisterUnaryServerInterceptors(intr ...grpc.UnaryServerInterceptor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		s.Fatal(errors.Reason("the server has already been started").Err())
	}
	s.unaryInterceptors = append(s.unaryInterceptors, intr...)
}

// RegisterStreamServerInterceptors registers grpc.StreamServerInterceptor's
// applied to all streaming RPCs that hit the server.
//
// Interceptors are chained in order they are registered, i.e. the first
// registered interceptor becomes the outermost. The initial chain already
// contains some base interceptors (e.g. for monitoring) and all interceptors
// registered by server modules. RegisterStreamServerInterceptors extends this
// chain. Subsequent calls to RegisterStreamServerInterceptors adds more
// interceptors into the chain.
//
// Must be called before Serve (panics otherwise).
func (s *Server) RegisterStreamServerInterceptors(intr ...grpc.StreamServerInterceptor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		s.Fatal(errors.Reason("the server has already been started").Err())
	}
	s.streamInterceptors = append(s.streamInterceptors, intr...)
}

// RegisterUnifiedServerInterceptors registers given interceptors into both
// unary and stream interceptor chains.
//
// It is just a convenience helper for UnifiedServerInterceptor's that usually
// need to be registered in both unary and stream interceptor chains. This
// method is equivalent to calling RegisterUnaryServerInterceptors and
// RegisterStreamServerInterceptors, passing corresponding flavors of
// interceptors to them.
//
// Must be called before Serve (panics otherwise).
func (s *Server) RegisterUnifiedServerInterceptors(intr ...grpcutil.UnifiedServerInterceptor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		s.Fatal(errors.Reason("the server has already been started").Err())
	}
	for _, cb := range intr {
		s.unaryInterceptors = append(s.unaryInterceptors, cb.Unary())
		s.streamInterceptors = append(s.streamInterceptors, cb.Stream())
	}
}

// ConfigurePRPC allows tweaking pRPC-specific server configuration.
//
// Use it only for changing pRPC-specific options (usually ones that are related
// to HTTP protocol in some way). This method **must not be used** for
// registering interceptors or setting authentication options (changes to them
// done here will cause a panic). Instead use RegisterUnaryServerInterceptors to
// register interceptors or SetRPCAuthMethods to change how the server
// authenticates RPC requests. Changes done through these methods will apply
// to both gRPC and pRPC servers.
//
// Must be called before Serve (panics otherwise).
func (s *Server) ConfigurePRPC(cb func(srv *prpc.Server)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		s.Fatal(errors.Reason("the server has already been started").Err())
	}
	cb(s.prpc)
	if s.prpc.UnaryServerInterceptor != nil {
		panic("use Server.RegisterUnaryServerInterceptors to register interceptors")
	}
}

// SetRPCAuthMethods overrides how the server authenticates incoming gRPC and
// pRPC requests.
//
// It receives a list of auth.Method implementations which will be applied
// one after another to try to authenticate the request until the first
// successful hit. If all methods end up to be non-applicable (i.e. none of the
// methods notice any headers they recognize), the request will be passed
// through to the handler as anonymous (coming from an "anonymous identity").
// Rejecting anonymous requests (if necessary) is the job of an authorization
// layer, often implemented as a gRPC interceptor. For simple cases use
// go.chromium.org/luci/server/auth/rpcacl interceptor.
//
// By default (if SetRPCAuthMethods is never called) the server will check
// incoming requests have an `Authorization` header with a Google OAuth2 access
// token that has `https://www.googleapis.com/auth/userinfo.email` scope (see
// auth.GoogleOAuth2Method). Requests without `Authorization` header will be
// considered anonymous.
//
// If OpenIDRPCAuthEnable option is set (matching `-open-id-rpc-auth-enable`
// flag), the service will recognize ID tokens as well. This is important for
// e.g. Cloud Run where this is the only authentication method supported
// natively by the platform. ID tokens are also generally faster to check than
// access tokens.
//
// Note that this call completely overrides the previously configured list of
// methods instead of appending to it, since chaining auth methods is often
// tricky and it is safer to just always provide the whole list at once.
//
// Passing an empty list of methods is allowed. All requests will be considered
// anonymous in that case.
//
// Note that this call **doesn't affect** how plain HTTP requests (hitting the
// main HTTP port and routed through s.Router) are authenticated. Very often
// RPC requests and plain HTTP requests need different authentication methods
// and using an RPC authentication for everything is incorrect. To authenticate
// plain HTTP requests use auth.Authenticate(...) HTTP router middleware,
// perhaps in combination with s.CookieAuth (which is non-nil if there is a
// server module installed that provides a cookie-based authentication scheme).
//
// Must be called before Serve (panics otherwise).
func (s *Server) SetRPCAuthMethods(methods []auth.Method) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		s.Fatal(errors.Reason("the server has already been started").Err())
	}
	s.rpcAuthMethods = methods
}

// Serve launches the serving loop.
//
// Blocks forever or until the server is stopped via Shutdown (from another
// goroutine or from a SIGTERM handler). Returns nil if the server was shutdown
// correctly or an error if it failed to start or unexpectedly died. The error
// is logged inside.
//
// Should be called only once. Panics otherwise.
func (s *Server) Serve() error {
	// Set s.started flag to "lock" the configuration. This would allow to read
	// fields like `s.ports` without the fear of a race conditions.
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		s.Fatal(errors.Reason("the server has already been started").Err())
	}
	s.started = true
	s.mu.Unlock()

	// The configuration is "locked" now and we can finish the setup.
	authInterceptor := auth.AuthenticatingInterceptor(s.rpcAuthMethods)

	// Assemble the final interceptor chains: base interceptors => auth =>
	// whatever was installed by users of server.Server. Note we put grpcmon
	// before the panic catcher to make sure panics are actually reported to
	// the monitoring. grpcmon is also before the authentication to make sure
	// auth errors are reported as well.
	unaryInterceptors := append([]grpc.UnaryServerInterceptor{
		grpcmon.UnaryServerInterceptor,
		grpcutil.UnaryServerPanicCatcherInterceptor,
		authInterceptor.Unary(),
	}, s.unaryInterceptors...)
	streamInterceptors := append([]grpc.StreamServerInterceptor{
		grpcmon.StreamServerInterceptor,
		grpcutil.StreamServerPanicCatcherInterceptor,
		authInterceptor.Stream(),
	}, s.streamInterceptors...)

	// Finish setting the pRPC server. It supports only unary RPCs. The root
	// request context is created in the HTTP land using base HTTP middlewares.
	s.prpc.UnaryServerInterceptor = grpcutil.ChainUnaryServerInterceptors(unaryInterceptors...)

	// Finish setting the gRPC server, if enabled.
	if s.grpcPort != nil {
		grpcRoot := s.grpcRoot()
		grpcDispatch := s.grpcDispatch()
		s.grpcPort.addServerOptions(
			grpc.ChainUnaryInterceptor(
				grpcRoot.Unary(),
				otelgrpc.UnaryServerInterceptor(),
				grpcDispatch.Unary(),
			),
			grpc.ChainUnaryInterceptor(unaryInterceptors...),
			grpc.ChainStreamInterceptor(
				grpcRoot.Stream(),
				otelgrpc.StreamServerInterceptor(),
				grpcDispatch.Stream(),
			),
			grpc.ChainStreamInterceptor(streamInterceptors...),
		)
	}

	// Run registered best-effort warmup callbacks right before serving.
	s.runWarmup()

	// Catch SIGTERM while inside the serving loop. Upon receiving SIGTERM, wait
	// until the pod is removed from the load balancer before actually shutting
	// down and refusing new connections. If we shutdown immediately, some clients
	// may see connection errors, because they are not aware yet the server is
	// closing: Pod shutdown sequence and Endpoints list updates are racing with
	// each other, we want Endpoints list updates to win, i.e. we want the pod to
	// actually be fully alive as long as it is still referenced in Endpoints
	// list. We can't guarantee this, but we can improve chances.
	stop := signals.HandleInterrupt(func() {
		if s.Options.Prod {
			s.waitUntilNotServing()
		}
		s.Shutdown()
	})
	defer stop()

	// Log how long it took from 'New' to the serving loop.
	logging.Infof(s.Context, "Startup done in %s", clock.Now(s.Context).Sub(s.startTime))

	// Unblock all pending RunInBackground goroutines, so they can start.
	close(s.ready)

	// Run serving loops in parallel.
	errs := make(errors.MultiError, len(s.ports))
	wg := sync.WaitGroup{}
	wg.Add(len(s.ports))
	for i, port := range s.ports {
		logging.Infof(s.Context, "Serving %s", port.nameForLog())
		i := i
		port := port
		go func() {
			defer wg.Done()
			if err := port.serve(func() context.Context { return s.Context }); err != nil {
				logging.WithError(err).Errorf(s.Context, "Server %s failed", port.nameForLog())
				errs[i] = err
				s.Shutdown() // close all other servers
			}
		}()
	}
	wg.Wait()

	// Per http.Server docs, we end up here *immediately* after Shutdown call was
	// initiated. Some requests can still be in-flight. We block until they are
	// done (as indicated by Shutdown call itself exiting).
	logging.Infof(s.Context, "Waiting for the server to stop...")
	<-s.done
	logging.Infof(s.Context, "The serving loop stopped, running the final cleanup...")
	s.runCleanup()
	logging.Infof(s.Context, "The server has stopped")

	if errs.First() != nil {
		return errs
	}
	return nil
}

// Shutdown gracefully stops the server if it was running.
//
// Blocks until the server is stopped. Can be called multiple times.
func (s *Server) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}

	logging.Infof(s.Context, "Shutting down the server...")

	// Tell all RunInBackground goroutines to stop.
	close(s.bgrDone)

	// Stop all http.Servers in parallel. Each Shutdown call blocks until the
	// corresponding server is stopped.
	wg := sync.WaitGroup{}
	wg.Add(len(s.ports))
	for _, port := range s.ports {
		port := port
		go func() {
			defer wg.Done()
			port.shutdown(s.Context)
		}()
	}
	wg.Wait()

	// Wait for all background goroutines to stop.
	s.bgrWg.Wait()

	// Notify Serve that it can exit now.
	s.stopped = true
	close(s.done)
}

// Fatal logs the error and immediately shuts down the process with exit code 3.
//
// No cleanup is performed. Deferred statements are not run. Not recoverable.
func (s *Server) Fatal(err error) {
	errors.Log(s.Context, err)
	os.Exit(3)
}

// healthResponse prepares text/plan response for the health check endpoints.
//
// It additionally contains some easy to obtain information that may help in
// debugging deployments.
func (s *Server) healthResponse(c context.Context) string {
	maybeEmpty := func(s string) string {
		if s == "" {
			return "<unknown>"
		}
		return s
	}
	return strings.Join([]string{
		"OK",
		"",
		"uptime:  " + clock.Now(c).Sub(s.startTime).String(),
		"image:   " + maybeEmpty(s.Options.ContainerImageID),
		"",
		"service: " + maybeEmpty(s.Options.TsMonServiceName),
		"job:     " + maybeEmpty(s.Options.TsMonJobName),
		"host:    " + s.Options.Hostname,
		"",
	}, "\n")
}

// waitUntilNotServing is called during the graceful shutdown and it tries to
// figure out when the traffic stops flowing to the server (i.e. when it is
// removed from the load balancer).
//
// It's a heuristic optimization for the case when the load balancer keeps
// sending traffic to a terminating Pod for some time after the Pod entered
// "Terminating" state. It can happen due to latencies in Endpoints list
// updates. We want to keep the listening socket open as long as there are
// incoming requests (but no longer than 1 min).
func (s *Server) waitUntilNotServing() {
	logging.Infof(s.Context, "Received SIGTERM, waiting for the traffic to stop...")

	// When the server is idle the loop below exits immediately and the server
	// enters the shutdown path, rejecting new connections. Since we gave
	// Kubernetes no time to update the Endpoints list, it is possible someone
	// still might send a request to the server (and it will be rejected).
	// To avoid that we always sleep a bit here to give Kubernetes a chance to
	// propagate the Endpoints list update everywhere. The loop below then
	// verifies clients got the update and stopped sending requests.
	time.Sleep(s.Options.ShutdownDelay)

	deadline := clock.Now(s.Context).Add(time.Minute)
	for {
		now := clock.Now(s.Context)
		lastReq, ok := s.lastReqTime.Load().(time.Time)
		if !ok || now.Sub(lastReq) > 15*time.Second {
			logging.Infof(s.Context, "No requests received in the last 15 sec, proceeding with the shutdown...")
			break
		}
		if now.After(deadline) {
			logging.Warningf(s.Context, "Gave up waiting for the traffic to stop, proceeding with the shutdown...")
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// RegisterWarmup registers a callback that is run in server's Serve right
// before the serving loop.
//
// It receives the global server context (including all customizations made
// by the user code in server.Main). Intended for best-effort warmups: there's
// no way to gracefully abort the server startup from a warmup callback.
//
// Registering a new warmup callback from within a warmup causes a deadlock,
// don't do that.
func (s *Server) RegisterWarmup(cb func(context.Context)) {
	s.warmupM.Lock()
	defer s.warmupM.Unlock()
	s.warmup = append(s.warmup, cb)
}

// runWarmup runs all registered warmup functions (sequentially in registration
// order).
func (s *Server) runWarmup() {
	s.warmupM.Lock()
	defer s.warmupM.Unlock()
	ctx := logging.SetField(s.Context, "activity", "luci.warmup")
	for _, cb := range s.warmup {
		cb(ctx)
	}
}

// RegisterCleanup registers a callback that is run in Serve after the server
// has exited the serving loop.
//
// Registering a new cleanup callback from within a cleanup causes a deadlock,
// don't do that.
func (s *Server) RegisterCleanup(cb func(context.Context)) {
	s.cleanupM.Lock()
	defer s.cleanupM.Unlock()
	s.cleanup = append(s.cleanup, cb)
}

// runCleanup runs all registered cleanup functions (sequentially in reverse
// order).
func (s *Server) runCleanup() {
	s.cleanupM.Lock()
	defer s.cleanupM.Unlock()
	for i := len(s.cleanup) - 1; i >= 0; i-- {
		s.cleanup[i](s.Context)
	}
}

// genUniqueBlob writes a pseudo-random byte blob into the given slice.
func (s *Server) genUniqueBlob(b []byte) {
	s.rndM.Lock()
	s.rnd.Read(b)
	s.rndM.Unlock()
}

// genUniqueID returns pseudo-random hex string of given even length.
func (s *Server) genUniqueID(l int) string {
	b := make([]byte, l/2)
	s.genUniqueBlob(b)
	return hex.EncodeToString(b)
}

// incomingRequest is a request received by the server.
//
// It is either an HTTP or a gRPC request.
type incomingRequest struct {
	url         string               // the full URL for logs
	method      string               // HTTP method verb for logs, e.g. "POST"
	metadata    auth.RequestMetadata // headers etc.
	healthCheck bool                 // true if this is a health check request
}

// requestResult is logged after completion of a request.
type requestResult struct {
	statusCode   int            // the HTTP status code to log
	requestSize  int64          // the request size in bytes if known
	responseSize int64          // the response size in bytes if known
	extraFields  logging.Fields // extra fields to log (will be mutated!)
}

// wrapHTTPHandler wraps port's router into net/http middlewares.
//
// TODO(vadimsh): Get rid of router.Middleware and move this to newRouter(...).
// Since introduction of http.Request.Context() there's no reason for
// router.Middleware to exist anymore.
func (s *Server) wrapHTTPHandler(next http.Handler) http.Handler {
	return s.httpRoot(
		otelhttp.NewHandler(
			s.httpDispatch(next),
			"",
			otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.URL.Path
			}),
		),
	)
}

// httpRoot is the entry point for non-gRPC HTTP requests.
//
// It is an http/net middleware for interoperability with other existing
// http/net middlewares (currently only OpenTelemetry otelhttp middleware).
//
// Its job is to initialize *incomingRequest in the context which is then
// examined by other middlewares (and the tracing sampler), in particular in
// httpDispatch.
//
// See grpcRoot(...) for a gRPC counterpart.
func (s *Server) httpRoot(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// This context is derived from s.Context (see Serve) and has various server
		// systems injected into it already. Its only difference from s.Context is
		// that http.Server cancels it when the client disconnects, which we want.
		ctx := r.Context()

		// Apply per-request HTTP timeout and corresponding context
		// cancellation cause, if any.
		var timeout time.Duration
		var cause error
		if strings.HasPrefix(r.URL.Path, "/internal/") {
			timeout = s.Options.InternalRequestTimeout
			cause = fmt.Errorf("request hit InternalRequestTimeout %v", timeout)
		} else {
			timeout = s.Options.DefaultRequestTimeout
			cause = fmt.Errorf("request hit DefaultRequestTimeout %v", timeout)
		}
		if timeout != 0 {
			var cancelCtx context.CancelFunc
			ctx, cancelCtx = context.WithTimeoutCause(ctx, timeout, cause)
			defer cancelCtx()
		}

		// Reconstruct the original URL for logging.
		protocol := r.Header.Get("X-Forwarded-Proto")
		if protocol != "https" {
			protocol = "http"
		}
		url := fmt.Sprintf("%s://%s%s", protocol, r.Host, r.RequestURI)

		// incomingRequest is used by middlewares that work with both HTTP and gRPC
		// requests, in particular it is used by startRequest(...).
		next.ServeHTTP(rw, r.WithContext(context.WithValue(ctx, &incomingRequestKey, &incomingRequest{
			url:         url,
			method:      r.Method,
			metadata:    auth.RequestMetadataForHTTP(r),
			healthCheck: r.RequestURI == healthEndpoint && isHealthCheckerUA(r.UserAgent()),
		})))
	})
}

// httpDispatch finishes HTTP request context initialization.
//
// Its primary purpose it so setup logging, but it also does some other context
// touches. See startRequest(...) where the bulk of work is happening.
//
// The next stop is the router.Middleware chain as registered in newRouter(...)
// and by the user code.
//
// See grpcDispatch(...) for a gRPC counterpart.
func (s *Server) httpDispatch(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Track how many response bytes are sent and what status is set, for logs.
		trackingRW := iotools.NewResponseWriter(rw)

		// Initialize per-request context (logging, GAE tickets, etc).
		ctx, done := s.startRequest(r.Context())

		// Log the result when done.
		defer func() {
			done(&requestResult{
				statusCode:   trackingRW.Status(),
				requestSize:  r.ContentLength,
				responseSize: trackingRW.ResponseSize(),
			})
		}()

		next.ServeHTTP(trackingRW, r.WithContext(ctx))
	})
}

// grpcRoot is the entry point for gRPC requests.
//
// Its job is to initialize *incomingRequest in the context which is then
// examined by other middlewares (and the tracing sampler), in particular in
// grpcDispatch.
//
// See httpRoot(...) for a HTTP counterpart.
func (s *Server) grpcRoot() grpcutil.UnifiedServerInterceptor {
	return func(ctx context.Context, fullMethod string, handler func(ctx context.Context) error) (err error) {
		// incomingRequest is used by middlewares that work with both HTTP and gRPC
		// requests, in particular it is used by startRequest(...).
		//
		// Note that here `ctx` is already derived from s.Context (except it is
		// canceled if the client disconnects). See grpcPort{} implementation.
		md := auth.RequestMetadataForGRPC(ctx)
		return handler(context.WithValue(ctx, &incomingRequestKey, &incomingRequest{
			url:         fmt.Sprintf("grpc://%s%s", md.Host(), fullMethod),
			method:      "POST",
			metadata:    md,
			healthCheck: strings.HasPrefix(fullMethod, "/grpc.health.") && isHealthCheckerUA(md.Header("User-Agent")),
		}))
	}
}

// grpcDispatch finishes gRPC request context initialization.
//
// Its primary purpose it so setup logging, but it also does some other context
// touches. See startRequest(...) where the bulk of work is happening.
//
// The next stop is the gRPC middleware chain as registered via server's API.
//
// See httpDispatch(...) for a HTTP counterpart.
func (s *Server) grpcDispatch() grpcutil.UnifiedServerInterceptor {
	return func(ctx context.Context, fullMethod string, handler func(ctx context.Context) error) (err error) {
		// Initialize per-request context (logging, GAE tickets, etc).
		ctx, done := s.startRequest(ctx)

		// Log the result when done.
		defer func() {
			code := status.Code(err)
			httpStatusCode := grpcutil.CodeStatus(code)

			// Log errors (for parity with pRPC server behavior).
			switch {
			case httpStatusCode >= 400 && httpStatusCode < 500:
				logging.Warningf(ctx, "%s", err)
			case httpStatusCode >= 500:
				logging.Errorf(ctx, "%s", err)
			}

			// Report canonical GRPC code as a log entry field for filtering by it.
			canonical, ok := codepb.Code_name[int32(code)]
			if !ok {
				canonical = fmt.Sprintf("%d", int64(code))
			}

			done(&requestResult{
				statusCode:  httpStatusCode, // this is an approximation
				extraFields: logging.Fields{"code": canonical},
			})
		}()

		return handler(ctx)
	}
}

// startRequest finishes preparing the per-request context.
//
// It returns a callback that must be called after finishing processing this
// request.
//
// The incoming context is assumed to be derived by either httpRoot(...) or
// grpcRoot(...) and have *incomingRequest inside.
func (s *Server) startRequest(ctx context.Context) (context.Context, func(*requestResult)) {
	// The value *must* be there. Let it panic if it is not.
	req := ctx.Value(&incomingRequestKey).(*incomingRequest)

	// If running on GAE, initialize the per-request API tickets needed to make
	// RPCs to the GAE service bridge.
	if s.Options.Serverless == module.GAE {
		ctx = gae.WithTickets(ctx, gae.RequestTickets(req.metadata))
	}

	// This is used in waitUntilNotServing.
	started := clock.Now(ctx)
	if !req.healthCheck {
		s.lastReqTime.Store(started)
	}

	// If the tracing is completely disabled we'll have an empty span context.
	// But we need a trace ID in the context anyway for correlating logs (see
	// below). Open a noop non-recording span with random generated trace ID.
	span := oteltrace.SpanFromContext(ctx)
	spanCtx := span.SpanContext()
	if !spanCtx.HasTraceID() {
		var traceID oteltrace.TraceID
		s.genUniqueBlob(traceID[:])
		spanCtx = oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
			TraceID: traceID,
		})
		ctx = oteltrace.ContextWithSpanContext(ctx, spanCtx)
	}

	// Associate all logs with one another by using the same trace ID, which also
	// matches the trace ID extracted by the propagator from incoming headers.
	// Make sure to use the full trace ID format that includes the project name.
	// This is important to group logs generated by us with logs generated by
	// the GCP (which uses the full trace ID) when running in Cloud. Outside of
	// Cloud it doesn't really matter what trace ID is used as long as all log
	// entries use the same one.
	traceID := spanCtx.TraceID().String()
	if s.Options.CloudProject != "" {
		traceID = fmt.Sprintf("projects/%s/traces/%s", s.Options.CloudProject, traceID)
	}

	// SpanID can be missing if there's no actual tracing. This is fine.
	spanID := ""
	if spanCtx.HasSpanID() {
		spanID = spanCtx.SpanID().String()
	}

	// When running in prod, make the logger emit log entries in JSON format that
	// Cloud Logger collectors understand natively.
	var severityTracker *sdlogger.SeverityTracker
	if s.Options.Prod {
		// Start assembling logging sink layers starting with the innermost one.
		logSink := s.stdout

		// If we are going to log the overall request status, install the tracker
		// that observes the maximum emitted severity to use it as an overall
		// severity for the request log entry.
		if s.logRequestCB != nil {
			severityTracker = &sdlogger.SeverityTracker{Out: logSink}
			logSink = severityTracker
		}

		// If have Cloud Error Reporting enabled, intercept errors to upload them.
		// TODO(vadimsh): Fill in `CloudErrorsSink.Request` with something.
		if s.errRptClient != nil {
			logSink = &sdlogger.CloudErrorsSink{
				Client: s.errRptClient,
				Out:    logSink,
			}
		}

		// Associate log entries with the tracing span where they were emitted.
		annotateWithSpan := func(ctx context.Context, e *sdlogger.LogEntry) {
			if spanID := oteltrace.SpanContextFromContext(ctx).SpanID(); spanID.IsValid() {
				e.SpanID = spanID.String()
			}
		}

		// Finally install all this into the request context.
		ctx = logging.SetFactory(ctx, sdlogger.Factory(logSink, sdlogger.LogEntry{
			TraceID:   traceID,
			Operation: &sdlogger.Operation{ID: s.genUniqueID(32)},
		}, annotateWithSpan))
	}

	// Do final context touches.
	ctx = caching.WithRequestCache(ctx)

	// This will be called once the request is fully processed.
	return ctx, func(res *requestResult) {
		now := clock.Now(ctx)
		latency := now.Sub(started)

		if req.healthCheck {
			// Do not log fast health check calls AT ALL, they just spam logs.
			if latency < healthTimeLogThreshold {
				return
			}
			// Emit a warning if the health check is slow, this likely indicates
			// high CPU load.
			logging.Warningf(ctx, "Health check is slow: %s > %s", latency, healthTimeLogThreshold)
		}

		// If there's no need to emit the overall request log entry, we are done.
		// See initLogging(...) for where this is decided.
		if s.logRequestCB == nil {
			return
		}

		// When running behind Envoy, log its request IDs to simplify debugging.
		extraFields := res.extraFields
		if xrid := req.metadata.Header("X-Request-Id"); xrid != "" {
			if extraFields == nil {
				extraFields = make(logging.Fields, 1)
			}
			extraFields["requestId"] = xrid
		}

		// If we were tracking the overall severity, collect the outcome.
		severity := sdlogger.InfoSeverity
		if severityTracker != nil {
			severity = severityTracker.MaxSeverity()
		}

		// Log the final outcome of the processed request.
		s.logRequestCB(ctx, &sdlogger.LogEntry{
			Severity:     severity,
			Timestamp:    sdlogger.ToTimestamp(now),
			TraceID:      traceID,
			TraceSampled: span.IsRecording(),
			SpanID:       spanID, // the top-level span ID if present
			Fields:       extraFields,
			RequestInfo: &sdlogger.RequestInfo{
				Method:       req.method,
				URL:          req.url,
				Status:       res.statusCode,
				RequestSize:  fmt.Sprintf("%d", res.requestSize),
				ResponseSize: fmt.Sprintf("%d", res.responseSize),
				UserAgent:    req.metadata.Header("User-Agent"),
				RemoteIP:     endUserIP(req.metadata),
				Latency:      fmt.Sprintf("%fs", latency.Seconds()),
			},
		})
	}
}

// initLogging initializes the server logging.
//
// Called very early during server startup process. Many server fields may not
// be initialized yet, be careful.
//
// When running in production uses the ugly looking JSON format that is hard to
// read by humans but which is parsed by google-fluentd and GCP serverless
// hosting environment.
//
// To support per-request log grouping in Cloud Logging UI there must be
// two different log streams:
//   - A stream with top-level HTTP request entries (conceptually like Apache's
//     access.log, i.e. with one log entry per request).
//   - A stream with logs produced within requests (correlated with HTTP request
//     logs via the trace ID field).
//
// Both streams are expected to have a particular format and use particular
// fields for Cloud Logging UI to display them correctly. This technique is
// primarily intended for GAE Flex, but it works in many Google environments:
// https://cloud.google.com/appengine/articles/logging#linking_app_logs_and_requests
//
// On GKE we use 'stderr' stream for top-level HTTP request entries and 'stdout'
// stream for logs produced by requests.
//
// On GAE and Cloud Run, the stream with top-level HTTP request entries is
// produced by the GCP runtime itself. So we emit only logs produced within
// requests (also to 'stdout', just like on GKE).
//
// In all environments 'stderr' stream is used to log all global activities that
// happens outside of any request handler (stuff like initialization, shutdown,
// background goroutines, etc).
//
// In non-production mode we use the human-friendly format and a single 'stderr'
// log stream for everything.
func (s *Server) initLogging() {
	if !s.Options.Prod {
		s.Context = gologger.StdConfig.Use(s.Context)
		s.Context = logging.SetLevel(s.Context, logging.Debug)
		s.logRequestCB = func(ctx context.Context, entry *sdlogger.LogEntry) {
			logging.Infof(ctx, "%d %s %q (%s)",
				entry.RequestInfo.Status,
				entry.RequestInfo.Method,
				entry.RequestInfo.URL,
				entry.RequestInfo.Latency,
			)
		}
		return
	}

	if s.Options.testStdout != nil {
		s.stdout = s.Options.testStdout
	} else {
		s.stdout = &sdlogger.Sink{Out: os.Stdout}
	}

	if s.Options.testStderr != nil {
		s.stderr = s.Options.testStderr
	} else {
		s.stderr = &sdlogger.Sink{Out: os.Stderr}
	}

	s.Context = logging.SetFactory(s.Context,
		sdlogger.Factory(s.stderr, sdlogger.LogEntry{
			Operation: &sdlogger.Operation{
				ID: s.genUniqueID(32), // correlate all global server logs together
			},
		}, nil),
	)
	s.Context = logging.SetLevel(s.Context, logging.Debug)

	// Skip writing the root request log entry on Serverless GCP since the load
	// balancer there writes the entry itself.
	switch s.Options.Serverless {
	case module.GAE:
		// Skip. GAE writes it to "appengine.googleapis.com/request_log" itself.
	case module.CloudRun:
		// Skip. Cloud Run writes it to "run.googleapis.com/requests" itself.
	default:
		// Emit to stderr where Cloud Logging collectors pick it up.
		s.logRequestCB = func(_ context.Context, entry *sdlogger.LogEntry) { s.stderr.Write(entry) }
	}
}

// initAuthStart initializes the core auth system by preparing the context
// and verifying auth tokens can actually be minted (i.e. supplied credentials
// are valid).
//
// It is called before the tsmon monitoring is initialized: tsmon needs auth.
// The rest of the auth initialization (the part that needs tsmon) happens in
// initAuthFinish after tsmon is initialized.
func (s *Server) initAuthStart() error {
	// Make a transport that appends information about the server as User-Agent.
	ua := s.Options.userAgent()
	rootTransport := clientauth.NewModifyingTransport(http.DefaultTransport, func(req *http.Request) error {
		newUA := ua
		if cur := req.UserAgent(); cur != "" {
			newUA += " " + cur
		}
		req.Header.Set("User-Agent", newUA)
		return nil
	})

	// Initialize the token generator based on s.Options.ClientAuth.
	opts := s.Options.ClientAuth

	// Use `rootTransport` for calls made by the token generator (e.g. when
	// refreshing tokens).
	opts.Transport = rootTransport

	// We aren't going to use the authenticator's transport (and thus its
	// monitoring), only the token source. DisableMonitoring == true removes some
	// log spam.
	opts.DisableMonitoring = true

	// GCP is very aggressive in caching the token internally (in the metadata
	// server) and refreshing it only when it is very close to its expiration. We
	// need to match this behavior in our in-process cache, otherwise
	// GetAccessToken complains that the token refresh procedure doesn't actually
	// change the token (because the metadata server returned the cached one).
	opts.MinTokenLifetime = 20 * time.Second

	// The default value for ClientAuth.SecretsDir is usually hardcoded to point
	// to where the token cache is located on developer machines (~/.config/...).
	// This location often doesn't exist when running from inside a container.
	// The token cache is also not really needed for production services that use
	// service accounts (they don't need cached refresh tokens). So in production
	// mode totally ignore default ClientAuth.SecretsDir and use whatever was
	// passed as -token-cache-dir. If it is empty (default), then no on-disk token
	// cache is used at all.
	//
	// If -token-cache-dir was explicitly set, always use it (even in dev mode).
	// This is useful when running containers locally: developer's credentials
	// on the host machine can be mounted inside the container.
	if s.Options.Prod || s.Options.TokenCacheDir != "" {
		opts.SecretsDir = s.Options.TokenCacheDir
	}

	// Annotate the context used for logging from the token generator.
	ctx := logging.SetField(s.Context, "activity", "luci.auth")
	tokens := clientauth.NewTokenGenerator(ctx, opts)

	// Prepare partially initialized structs for the auth.Config. They will be
	// fully initialized in initAuthFinish once we have a sufficiently working
	// auth context that can call Cloud IAM.
	s.signer = &signerImpl{srv: s}
	s.actorTokens = &actorTokensImpl{}

	// Either use the explicitly passed AuthDB provider or the one initialized
	// by initAuthDB.
	provider := s.Options.AuthDBProvider
	if provider == nil {
		provider = func(context.Context) (authdb.DB, error) {
			db, _ := s.authDB.Load().(authdb.DB) // refreshed asynchronously in refreshAuthDB
			return db, nil
		}
	}

	// Initialize the state in the context.
	s.Context = auth.Initialize(s.Context, &auth.Config{
		DBProvider: provider,
		Signer:     s.signer,
		AccessTokenProvider: func(ctx context.Context, scopes []string) (*oauth2.Token, error) {
			return tokens.GenerateOAuthToken(ctx, scopes, 0)
		},
		IDTokenProvider: func(ctx context.Context, audience string) (*oauth2.Token, error) {
			return tokens.GenerateIDToken(ctx, audience, 0)
		},
		ActorTokensProvider: s.actorTokens,
		AnonymousTransport:  func(context.Context) http.RoundTripper { return rootTransport },
		FrontendClientID:    func(context.Context) (string, error) { return s.Options.FrontendClientID, nil },
		EndUserIP:           endUserIP,
		IsDevMode:           !s.Options.Prod,
	})

	// Note: we initialize a token source for one arbitrary set of scopes here. In
	// many practical cases this is sufficient to verify that credentials are
	// valid. For example, when we use service account JSON key, if we can
	// generate a token with *some* scope (meaning Cloud accepted our signature),
	// we can generate tokens with *any* scope, since there's no restrictions on
	// what scopes are accessible to a service account, as long as the private key
	// is valid (which we just verified by generating some token).
	_, err := tokens.GenerateOAuthToken(ctx, auth.CloudOAuthScopes, 0)
	if err != nil {
		// ErrLoginRequired may happen only when running the server locally using
		// developer's credentials. Let them know how the problem can be fixed.
		if !s.Options.Prod && err == clientauth.ErrLoginRequired {
			scopes := fmt.Sprintf("-scopes %q", strings.Join(s.Options.ClientAuth.Scopes, " "))
			if opts.ActAsServiceAccount != "" && opts.ActViaLUCIRealm == "" {
				scopes = "-scopes-iam"
			}
			logging.Errorf(s.Context, "Looks like you run the server locally and it doesn't have credentials for some OAuth scopes")
			logging.Errorf(s.Context, "Run the following command to set them up: ")
			logging.Errorf(s.Context, "  $ luci-auth login %s", scopes)
		}
		return errors.Annotate(err, "failed to initialize the token source").Err()
	}

	// Report who we are running as. Useful when debugging access issues.
	switch email, err := tokens.GetEmail(); {
	case err == nil:
		logging.Infof(s.Context, "Running as %s", email)
		s.runningAs = email
	case err == clientauth.ErrNoEmail:
		logging.Warningf(s.Context, "Running as <unknown>, cautiously proceeding...")
	case err != nil:
		return errors.Annotate(err, "failed to check the service account email").Err()
	}

	return nil
}

// initAuthFinish finishes auth system initialization.
//
// It is called after tsmon is initialized.
func (s *Server) initAuthFinish() error {
	// We should be able to make basic authenticated requests now and can
	// construct a token source used by server's own guts to call Cloud APIs,
	// such us Cloud Trace and Cloud Error Reporting (and others).
	var err error
	s.cloudTS, err = auth.GetTokenSource(s.Context, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return errors.Annotate(err, "failed to initialize the cloud token source").Err()
	}

	// Finish constructing `signer` and `actorTokens` that were waiting for
	// an IAM client.
	iamClient, err := credentials.NewIamCredentialsClient(
		s.Context,
		option.WithTokenSource(s.cloudTS),
		option.WithGRPCDialOption(grpc.WithStatsHandler(&grpcmon.ClientRPCStatsMonitor{})),
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor())),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor())),
	)
	if err != nil {
		return errors.Annotate(err, "failed to construct IAM client").Err()
	}
	s.RegisterCleanup(func(ctx context.Context) { iamClient.Close() })
	s.signer.iamClient = iamClient
	s.actorTokens.iamClient = iamClient

	// If not using a custom AuthDB provider, initialize the standard one that
	// fetches AuthDB (a database with groups and auth config) from a central
	// place. This also starts a goroutine to periodically refresh it.
	if s.Options.AuthDBProvider == nil {
		if err := s.initAuthDB(); err != nil {
			return errors.Annotate(err, "failed to initialize AuthDB").Err()
		}
	}

	// Default RPC authentication methods. See also SetRPCAuthMethods.
	s.rpcAuthMethods = make([]auth.Method, 0, 2)
	if s.Options.OpenIDRPCAuthEnable {
		// The preferred authentication method.
		s.rpcAuthMethods = append(s.rpcAuthMethods, &openid.GoogleIDTokenAuthMethod{
			AudienceCheck: openid.AudienceMatchesHost,
			Audience:      s.Options.OpenIDRPCAuthAudience,
			SkipNonJWT:    true, // pass OAuth2 access tokens through
		})
	}
	// Backward compatibility for the RPC Explorer and old clients.
	s.rpcAuthMethods = append(s.rpcAuthMethods, &auth.GoogleOAuth2Method{
		Scopes: []string{clientauth.OAuthScopeEmail},
	})

	return nil
}

// initAuthDB interprets -auth-db-* flags and sets up fetching of AuthDB.
func (s *Server) initAuthDB() error {
	// Check flags are compatible.
	switch {
	case s.Options.AuthDBPath != "" && s.Options.AuthServiceHost != "":
		return errors.Reason("-auth-db-path and -auth-service-host can't be used together").Err()
	case s.Options.AuthServiceHost == "" && (s.Options.AuthDBDump != "" || s.Options.AuthDBSigner != ""):
		return errors.Reason("-auth-db-dump and -auth-db-signer can be used only with -auth-service-host").Err()
	case s.Options.AuthDBDump != "" && !strings.HasPrefix(s.Options.AuthDBDump, "gs://"):
		return errors.Reason("-auth-db-dump value should start with gs://, got %q", s.Options.AuthDBDump).Err()
	case strings.Contains(s.Options.AuthServiceHost, "/"):
		return errors.Reason("-auth-service-host should be a plain hostname, got %q", s.Options.AuthServiceHost).Err()
	}

	// Fill in defaults.
	if s.Options.AuthServiceHost != "" {
		if s.Options.AuthDBDump == "" {
			s.Options.AuthDBDump = fmt.Sprintf("gs://%s/auth-db", s.Options.AuthServiceHost)
		}
		if s.Options.AuthDBSigner == "" {
			if !strings.HasSuffix(s.Options.AuthServiceHost, ".appspot.com") {
				return errors.Reason("-auth-db-signer is required if -auth-service-host is not *.appspot.com").Err()
			}
			s.Options.AuthDBSigner = fmt.Sprintf("%s@appspot.gserviceaccount.com",
				strings.TrimSuffix(s.Options.AuthServiceHost, ".appspot.com"))
		}
	}

	// Fetch the initial copy of AuthDB. Note that this happens before we start
	// the serving loop, to make sure incoming requests have some AuthDB to use.
	if err := s.refreshAuthDB(s.Context); err != nil {
		return errors.Annotate(err, "failed to load the initial AuthDB version").Err()
	}

	// Periodically refresh it in the background.
	s.RunInBackground("luci.authdb", func(c context.Context) {
		for {
			jitter := time.Duration(rand.Int63n(int64(10 * time.Second)))
			if r := <-clock.After(c, 30*time.Second+jitter); r.Err != nil {
				return // the context is canceled
			}
			if err := s.refreshAuthDB(c); err != nil {
				// Don't log the error if the server is shutting down.
				if !errors.Is(err, context.Canceled) {
					logging.WithError(err).Errorf(c, "Failed to reload AuthDB, using the cached one")
				}
			}
		}
	})
	return nil
}

// refreshAuthDB reloads AuthDB from the source and stores it in memory.
func (s *Server) refreshAuthDB(c context.Context) error {
	cur, _ := s.authDB.Load().(authdb.DB)
	db, err := s.fetchAuthDB(c, cur)
	if err != nil {
		return err
	}
	s.authDB.Store(db)
	return nil
}

// fetchAuthDB fetches the most recent copy of AuthDB from the external source.
//
// Used only if Options.AuthDBProvider is nil.
//
// 'cur' is the currently used AuthDB or nil if fetching it for the first time.
// Returns 'cur' as is if it's already fresh.
func (s *Server) fetchAuthDB(c context.Context, cur authdb.DB) (authdb.DB, error) {
	// Loading from a local file (useful in integration tests).
	if s.Options.AuthDBPath != "" {
		r, err := os.Open(s.Options.AuthDBPath)
		if err != nil {
			return nil, errors.Annotate(err, "failed to open AuthDB file").Err()
		}
		defer r.Close()
		db, err := authdb.SnapshotDBFromTextProto(r)
		if err != nil {
			return nil, errors.Annotate(err, "failed to load AuthDB file").Err()
		}
		return db, nil
	}

	// Loading from a GCS dump (s.Options.AuthDB* are validated here already).
	if s.Options.AuthDBDump != "" {
		c, cancel := clock.WithTimeout(c, 5*time.Minute)
		defer cancel()
		fetcher := dump.Fetcher{
			StorageDumpPath:    s.Options.AuthDBDump[len("gs://"):],
			AuthServiceURL:     "https://" + s.Options.AuthServiceHost,
			AuthServiceAccount: s.Options.AuthDBSigner,
			OAuthScopes:        auth.CloudOAuthScopes,
		}
		curSnap, _ := cur.(*authdb.SnapshotDB)
		snap, err := fetcher.FetchAuthDB(c, curSnap)
		if err != nil {
			return nil, errors.Annotate(err, "fetching from GCS dump failed").Err()
		}
		return snap, nil
	}

	// In dev mode default to "allow everything".
	if !s.Options.Prod {
		return authdb.DevServerDB{}, nil
	}

	// In prod mode default to "fail on any non-trivial check". Some services may
	// not need to use AuthDB at all and configuring it for them is a hassle. If
	// they try to use it for something vital, they'll see the error.
	return authdb.UnconfiguredDB{
		Error: errors.Reason("a source of AuthDB is not configured, see -auth-* server flags").Err(),
	}, nil
}

// initTSMon initializes time series monitoring state.
func (s *Server) initTSMon() error {
	// We keep tsmon always enabled (flushing to /dev/null if no -ts-mon-* flags
	// are set) so that tsmon's in-process store is populated, and metrics there
	// can be examined via /admin/tsmon. This is useful when developing/debugging
	// tsmon metrics.
	var customMonitor monitor.Monitor
	if s.Options.TsMonAccount == "" || s.Options.TsMonServiceName == "" || s.Options.TsMonJobName == "" {
		logging.Infof(s.Context, "tsmon is in the debug mode: metrics are collected, but flushed to /dev/null (pass -ts-mon-* flags to start uploading metrics)")
		customMonitor = monitor.NewNilMonitor()
	}

	interval := int(s.Options.TsMonFlushInterval.Seconds())
	if interval == 0 {
		interval = int(defaultTsMonFlushInterval.Seconds())
	}
	timeout := int(s.Options.TsMonFlushTimeout.Seconds())
	if timeout == 0 {
		timeout = int(defaultTsMonFlushTimeout.Seconds())
	}
	if timeout >= interval {
		return errors.Reason("-ts-mon-flush-timeout (%ds) must be shorter than -ts-mon-flush-interval (%ds)", timeout, interval).Err()
	}
	s.tsmon = &tsmon.State{
		CustomMonitor: customMonitor,
		Settings: &tsmon.Settings{
			Enabled:            true,
			ProdXAccount:       s.Options.TsMonAccount,
			FlushIntervalSec:   interval,
			FlushTimeoutSec:    timeout,
			ReportRuntimeStats: true,
		},
		Target: func(c context.Context) target.Task {
			// TODO(vadimsh): We pretend to be a GAE app for now to be able to
			// reuse existing dashboards. Each pod pretends to be a separate GAE
			// version. That way we can stop worrying about TaskNumAllocator and just
			// use 0 (since there'll be only one task per "version"). This looks
			// chaotic for deployments with large number of pods.
			return target.Task{
				DataCenter:  "appengine",
				ServiceName: s.Options.TsMonServiceName,
				JobName:     s.Options.TsMonJobName,
				HostName:    s.Options.Hostname,
			}
		},
	}
	if customMonitor != nil {
		tsmon.PortalPage.SetReadOnlySettings(s.tsmon.Settings,
			"Running in the debug mode. Pass all -ts-mon-* command line flags to start uploading metrics.")
	} else {
		tsmon.PortalPage.SetReadOnlySettings(s.tsmon.Settings,
			"Settings are controlled through -ts-mon-* command line flags.")
	}

	// Enable this configuration in s.Context so all transports created during
	// the server startup have tsmon instrumentation.
	s.tsmon.Activate(s.Context)

	// Report our image version as a metric, useful to monitor rollouts.
	tsmoncommon.RegisterCallbackIn(s.Context, func(ctx context.Context) {
		versionMetric.Set(ctx, s.Options.ImageVersion())
	})

	// Periodically flush metrics.
	s.RunInBackground("luci.tsmon", s.tsmon.FlushPeriodically)
	return nil
}

// otelResource returns an OTEL resource identifying this server instance.
//
// It is just a bunch of labels essentially reported to monitoring backends
// together with traces.
func (s *Server) otelResource(ctx context.Context) (*resource.Resource, error) {
	return resource.New(
		ctx,
		resource.WithTelemetrySDK(),
		resource.WithDetectors(gcp.NewDetector()),
		resource.WithAttributes(
			semconv.ServiceName(fmt.Sprintf("%s/%s", s.Options.TsMonServiceName, s.Options.TsMonJobName)),
			semconv.ServiceInstanceID(s.Options.Hostname),
			semconv.ContainerImageName(s.Options.ImageName()),
			semconv.ContainerImageTag(s.Options.ImageVersion()),
		),
	)
}

// otelErrorHandler returns a top-level OTEL error catcher.
//
// It just logs errors (with some dedupping to avoid spam).
func (s *Server) otelErrorHandler(ctx context.Context) otel.ErrorHandlerFunc {
	// State for suppressing repeated ResourceExhausted error messages, otherwise
	// logs may get flooded with them. They are usually not super important, but
	// ignoring them completely is also not great.
	errorDedup := struct {
		lock   sync.Mutex
		report time.Time
		count  int
	}{}
	return func(err error) {
		if !strings.Contains(err.Error(), "ResourceExhausted") {
			logging.Warningf(ctx, "Error in Cloud Trace exporter: %s", err)
			return
		}

		errorDedup.lock.Lock()
		defer errorDedup.lock.Unlock()

		errorDedup.count++

		if errorDedup.report.IsZero() || time.Since(errorDedup.report) > 5*time.Minute {
			if errorDedup.report.IsZero() {
				logging.Warningf(ctx, "Error in Cloud Trace exporter: %s", err)
			} else {
				logging.Warningf(ctx, "Error in Cloud Trace exporter: %s (%d occurrences in %s since the last report)", err, errorDedup.count, time.Since(errorDedup.report))
			}
			errorDedup.report = time.Now()
			errorDedup.count = 0
		}
	}
}

// otelSampler prepares a sampler based on CLI flags and environment.
func (s *Server) otelSampler(ctx context.Context) (trace.Sampler, error) {
	// On GCP Serverless let the GCP load balancer make decisions about
	// sampling. If it decides to sample a trace, it will let us know through
	// options of the parent span in X-Cloud-Trace-Context. We will collect only
	// traces from requests that GCP wants to sample itself. Traces without
	// a parent context are never sampled. This also means traces from random
	// background goroutines aren't sampled either (i.e. we don't need GateSampler
	// as used below).
	if s.Options.Serverless.IsGCP() {
		logging.Infof(ctx, "Setting up Cloud Trace exports to %q using GCP Serverless sampling strategy", s.Options.CloudProject)
		return trace.ParentBased(trace.NeverSample()), nil
	}

	// Parse -trace-sampling spec to get the base sampler.
	sampling := s.Options.TraceSampling
	if sampling == "" {
		sampling = "0.1qps"
	}
	logging.Infof(ctx, "Setting up Cloud Trace exports to %q (%s)", s.Options.CloudProject, sampling)
	sampler, err := internal.BaseSampler(sampling)
	if err != nil {
		return nil, errors.Annotate(err, "bad -trace-sampling").Err()
	}

	// Sample only if the context is an incoming request context. This is needed
	// to avoid various background goroutines spamming with top-level spans. This
	// usually happens if a library is oblivious of tracing, but uses an
	// instrumented HTTP or gRPC client it got from outside, and the passes
	// context.Background() (or some unrelated context) to it. The end result is
	// lots and lots of non-informative disconnected top-level spans.
	//
	// Also skip sampling health check requests, they end up being spammy as well.
	sampler = internal.GateSampler(sampler, func(ctx context.Context) bool {
		req, _ := ctx.Value(&incomingRequestKey).(*incomingRequest)
		return req != nil && !req.healthCheck
	})

	// Inherit the sampling decision from a parent span. Note this totally ignores
	// `sampler` if there's a parent span (local or remote). This is usually what
	// we want to get complete trace trees with well-defined root and no gaps.
	return trace.ParentBased(sampler), nil
}

// otelSpanExporter initializes a trace spans exporter.
func (s *Server) otelSpanExporter(ctx context.Context) (trace.SpanExporter, error) {
	return texporter.New(
		texporter.WithContext(ctx),
		texporter.WithProjectID(s.Options.CloudProject),
		texporter.WithTraceClientOptions([]option.ClientOption{
			option.WithTokenSource(s.cloudTS),
		}),
	)
}

// initOpenTelemetry initializes OpenTelemetry to export to Cloud Trace.
func (s *Server) initOpenTelemetry() error {
	// Initialize a transformer that knows how to extract span info from the
	// context and serialize it as a bunch of headers and vice-versa. It is
	// invoked by otelhttp and otelgrpc middleware and when creating instrumented
	// HTTP clients. Recognize X-Cloud-Trace-Context for compatibility with traces
	// created by GCLB.
	//
	// It is used to parse incoming headers even when tracing is disabled, so
	// initialize it unconditionally, just don't install as a global propagator.
	s.propagator = propagation.NewCompositeTextMapPropagator(
		gcppropagator.CloudTraceOneWayPropagator{},
		propagation.TraceContext{},
	)

	// If tracing is disabled, initialize OTEL with noop providers that just
	// silently discard everything. Otherwise OTEL leaks memory, see
	// https://github.com/open-telemetry/opentelemetry-go-contrib/issues/5190
	if !s.Options.shouldEnableTracing() {
		otel.SetTracerProvider(oteltracenoop.TracerProvider{})
		otel.SetMeterProvider(otelmetricnoop.MeterProvider{})
		return nil
	}

	// Annotate logs from OpenTelemetry so they can be filtered in Cloud Logging.
	ctx := logging.SetField(s.Context, "activity", "luci.trace")

	// TODO(vadimsh): Install OpenTelemetry global logger using otel.SetLogger().
	// This will require implementing a hefty logr.LogSink interface on top of
	// the LUCI logger. Not doing that results in garbled stderr when OTEL wants
	// to log something (unclear when it happens exactly, if at all).

	res, err := s.otelResource(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to init OpenTelemetry resource").Err()
	}
	sampler, err := s.otelSampler(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to init OpenTelemetry sampler").Err()
	}
	exp, err := s.otelSpanExporter(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to init OpenTelemetry span exporter").Err()
	}

	tp := trace.NewTracerProvider(
		trace.WithResource(res),
		trace.WithSampler(sampler),
		trace.WithBatcher(exp,
			trace.WithMaxQueueSize(8192),           // how much to buffer before dropping
			trace.WithBatchTimeout(30*time.Second), // how long to buffer before flushing
			trace.WithExportTimeout(time.Minute),   // deadline for the export RPC call
			trace.WithMaxExportBatchSize(2048),     // size of a single RPC
		),
	)

	s.RegisterCleanup(func(ctx context.Context) {
		ctx = logging.SetField(ctx, "activity", "luci.trace")
		if err := tp.ForceFlush(ctx); err != nil {
			logging.Errorf(ctx, "Final trace flush failed: %s", err)
		}
		if err := tp.Shutdown(ctx); err != nil {
			logging.Errorf(ctx, "Error shutting down TracerProvider: %s", err)
		}
	})

	// Register all globals to make them be used by default.
	otel.SetErrorHandler(s.otelErrorHandler(ctx))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(s.propagator)

	// We don't use OTEL metrics. Set the noop provider to avoid leaking memory.
	// See https://github.com/open-telemetry/opentelemetry-go-contrib/issues/5190.
	otel.SetMeterProvider(otelmetricnoop.MeterProvider{})

	return nil
}

// initProfiling initialized Cloud Profiler.
func (s *Server) initProfiling() error {
	// Skip if not enough configuration is given.
	switch {
	case !s.Options.Prod:
		return nil // silently skip, no need for log spam in dev mode
	case s.Options.CloudProject == "":
		logging.Infof(s.Context, "Cloud Profiler is disabled: -cloud-project is not set")
		return nil
	case s.Options.ProfilingServiceID == "" && s.Options.TsMonJobName == "":
		logging.Infof(s.Context, "Cloud Profiler is disabled: neither -profiling-service-id nor -ts-mon-job-name are set")
		return nil
	}

	// Enable profiler based on a given probability. Low probabilities are useful
	// to avoid hitting Cloud Profiler quotas when running services with many
	// replicas. Profiles are aggregated anyway, for large enough number of
	// servers it doesn't matter if only a random subset of them is sampled.
	sample := rand.Float64()
	if sample < s.Options.ProfilingProbability {
		if s.Options.ProfilingProbability >= 1.0 {
			logging.Infof(s.Context, "Cloud Profiler is enabled")
		} else {
			logging.Infof(s.Context,
				"Cloud Profiler is enabled: rand %.2f < profiling-probability %.2f",
				sample, s.Options.ProfilingProbability)
		}
	} else {
		if s.Options.ProfilingProbability <= 0 {
			logging.Infof(s.Context, "Cloud Profiler is disabled")
		} else {
			logging.Infof(s.Context,
				"Cloud Profiler is disabled: rand %.2f >= profiling-probability %.2f",
				sample, s.Options.ProfilingProbability)
		}
		return nil
	}

	cfg := profiler.Config{
		ProjectID:      s.Options.CloudProject,
		Service:        s.getServiceID(),
		ServiceVersion: s.Options.ImageVersion(),
		Instance:       s.Options.Hostname,
		// Note: these two options may potentially have impact on performance, but
		// it is likely small enough not to bother.
		MutexProfiling: true,
		AllocForceGC:   true,
	}

	// Launch the agent that runs in the background and periodically collects and
	// uploads profiles. It fails to launch if Service or ServiceVersion do not
	// pass regexp validation. Make it non-fatal, but still log.
	if err := profiler.Start(cfg, option.WithTokenSource(s.cloudTS)); err != nil {
		logging.Errorf(s.Context, "Cloud Profiler is disabled: failed do start - %s", err)
		return nil
	}

	logging.Infof(s.Context, "Set up Cloud Profiler (service %q, version %q)", cfg.Service, cfg.ServiceVersion)
	return nil
}

// getServiceID get the service id from either ProfilingServiceID or TsMonJobName.
func (s *Server) getServiceID() string {
	// Prefer ProfilingServiceID if given, fall back to TsMonJobName. Replace
	// forbidden '/' symbol.
	serviceID := s.Options.ProfilingServiceID
	if serviceID == "" {
		serviceID = s.Options.TsMonJobName
	}
	serviceID = strings.ReplaceAll(serviceID, "/", "-")
	return serviceID
}

// initMainPort initializes the server on options.HTTPAddr port.
func (s *Server) initMainPort() error {
	var err error
	s.mainPort, err = s.AddPort(PortOptions{
		Name:       "main",
		ListenAddr: s.Options.HTTPAddr,
	})
	if err != nil {
		return err
	}
	s.Routes = s.mainPort.Routes

	// Install auth info handlers (under "/auth/api/v1/server/").
	auth.InstallHandlers(s.Routes, nil)

	// Prepare the pRPC server. Its configuration will be finished in Serve after
	// all interceptors and authentication methods are registered.
	s.prpc = &prpc.Server{
		// Allow compression when not running on GAE. On GAE compression for text
		// responses is done by GAE itself and doing it in our code would be
		// wasteful.
		EnableResponseCompression: s.Options.Serverless != module.GAE,
	}
	discovery.Enable(s.prpc)
	s.prpc.InstallHandlers(s.Routes, nil)

	return nil
}

// initGrpcPort initializes the listening gRPC port.
func (s *Server) initGrpcPort() error {
	if s.Options.GRPCAddr == "" || s.Options.GRPCAddr == "-" {
		return nil // the gRPC port is disabled
	}
	listener, err := s.createListener(s.Options.GRPCAddr)
	if err != nil {
		return errors.Annotate(err, `failed to bind the listening port for "grpc" at %q`, s.Options.GRPCAddr).Err()
	}
	s.grpcPort = &grpcPort{listener: listener}
	s.ports = append(s.ports, s.grpcPort)
	return nil
}

// initAdminPort initializes the server on options.AdminAddr port.
func (s *Server) initAdminPort() error {
	if s.Options.AdminAddr == "-" {
		return nil // the admin port is disabled
	}

	// Admin portal uses XSRF tokens that require a secret key. We generate this
	// key randomly during process startup (i.e. now). It means XSRF tokens in
	// admin HTML pages rendered by a server process are understood only by the
	// exact same process. This is OK for admin pages (they are not behind load
	// balancers and we don't care that a server restart invalidates all tokens).
	secret := make([]byte, 20)
	if _, err := cryptorand.Read(secret); err != nil {
		return err
	}
	store := secrets.NewDerivedStore(secrets.Secret{Active: secret})
	withAdminSecret := router.NewMiddlewareChain(func(c *router.Context, next router.Handler) {
		c.Request = c.Request.WithContext(secrets.Use(c.Request.Context(), store))
		next(c)
	})

	// Install endpoints accessible through the admin port only.
	adminPort, err := s.AddPort(PortOptions{
		Name:           "admin",
		ListenAddr:     s.Options.AdminAddr,
		DisableMetrics: true, // do not pollute HTTP metrics with admin-only routes
	})
	if err != nil {
		return err
	}
	routes := adminPort.Routes

	routes.GET("/", nil, func(c *router.Context) {
		http.Redirect(c.Writer, c.Request, "/admin/portal", http.StatusFound)
	})
	portal.InstallHandlers(routes, withAdminSecret, portal.AssumeTrustedPort)

	// Install pprof endpoints on the admin port. Note that they must not be
	// exposed via the main serving port, since they do no authentication and
	// may leak internal information. Also note that pprof handlers rely on
	// routing structure not supported by our router, so we do a bit of manual
	// routing.
	//
	// See also internal/pprof.go for more profiling goodies exposed through the
	// admin portal.
	routes.GET("/debug/pprof/*path", nil, func(c *router.Context) {
		switch strings.TrimPrefix(c.Params.ByName("path"), "/") {
		case "cmdline":
			pprof.Cmdline(c.Writer, c.Request)
		case "profile":
			pprof.Profile(c.Writer, c.Request)
		case "symbol":
			pprof.Symbol(c.Writer, c.Request)
		case "trace":
			pprof.Trace(c.Writer, c.Request)
		default:
			pprof.Index(c.Writer, c.Request)
		}
	})
	return nil
}

// initErrorReporting initializes an Error Report client.
func (s *Server) initErrorReporting() error {
	if !s.Options.CloudErrorReporting || s.Options.CloudProject == "" {
		return nil
	}

	// Get token source to call Error Reporting API.
	var err error
	s.errRptClient, err = errorreporting.NewClient(s.Context, s.Options.CloudProject, errorreporting.Config{
		ServiceName:    s.getServiceID(),
		ServiceVersion: s.Options.ImageVersion(),
		OnError: func(err error) {
			// TODO(crbug/1204640): s/Warningf/Errorf once "Error Reporting" is itself
			// more reliable.
			logging.Warningf(s.Context, "Error Reporting could not log error: %s", err)
		},
	}, option.WithTokenSource(s.cloudTS))
	if err != nil {
		return err
	}

	s.RegisterCleanup(func(ctx context.Context) { s.errRptClient.Close() })
	return nil
}

// initWarmup schedules execution of global warmup callbacks.
//
// On GAE also registers /_ah/warmup route.
func (s *Server) initWarmup() error {
	// See https://cloud.google.com/appengine/docs/standard/go/configuring-warmup-requests.
	// All warmups should happen *before* the serving loop and /_ah/warmup should
	// just always return OK.
	if s.Options.Serverless == module.GAE {
		s.Routes.GET("/_ah/warmup", nil, func(*router.Context) {})
	}
	s.RegisterWarmup(func(ctx context.Context) { warmup.Warmup(ctx) })
	return nil
}

// signerImpl implements signing.Signer on top of *Server.
type signerImpl struct {
	srv       *Server
	iamClient *credentials.IamCredentialsClient
}

// SignBytes signs the blob with some active private key.
func (s *signerImpl) SignBytes(ctx context.Context, blob []byte) (keyName string, signature []byte, err error) {
	resp, err := s.iamClient.SignBlob(ctx, &credentialspb.SignBlobRequest{
		Name:    "projects/-/serviceAccounts/" + s.srv.runningAs,
		Payload: blob,
	})
	if err != nil {
		return "", nil, grpcutil.WrapIfTransient(err)
	}
	return resp.KeyId, resp.SignedBlob, nil
}

// Certificates returns a bundle with public certificates for all active keys.
func (s *signerImpl) Certificates(ctx context.Context) (*signing.PublicCertificates, error) {
	return signing.FetchCertificatesForServiceAccount(ctx, s.srv.runningAs)
}

// ServiceInfo returns information about the current service.
func (s *signerImpl) ServiceInfo(ctx context.Context) (*signing.ServiceInfo, error) {
	return &signing.ServiceInfo{
		AppID:              s.srv.Options.CloudProject,
		AppRuntime:         "go",
		AppRuntimeVersion:  runtime.Version(),
		AppVersion:         s.srv.Options.ImageVersion(),
		ServiceAccountName: s.srv.runningAs,
	}, nil
}

// actorTokensImpl implements auth.ActorTokensProvider using IAM Credentials.
type actorTokensImpl struct {
	iamClient *credentials.IamCredentialsClient
}

// GenerateAccessToken generates an access token for the given account.
func (a *actorTokensImpl) GenerateAccessToken(ctx context.Context, serviceAccount string, scopes, delegates []string) (*oauth2.Token, error) {
	resp, err := a.iamClient.GenerateAccessToken(ctx, &credentialspb.GenerateAccessTokenRequest{
		Name:      "projects/-/serviceAccounts/" + serviceAccount,
		Scope:     scopes,
		Delegates: delegatesList(delegates),
	})
	if err != nil {
		return nil, grpcutil.WrapIfTransient(err)
	}
	return &oauth2.Token{
		AccessToken: resp.AccessToken,
		TokenType:   "Bearer",
		Expiry:      resp.ExpireTime.AsTime(),
	}, nil
}

// GenerateIDToken generates an ID token for the given account.
func (a *actorTokensImpl) GenerateIDToken(ctx context.Context, serviceAccount, audience string, delegates []string) (string, error) {
	resp, err := a.iamClient.GenerateIdToken(ctx, &credentialspb.GenerateIdTokenRequest{
		Name:         "projects/-/serviceAccounts/" + serviceAccount,
		Audience:     audience,
		Delegates:    delegatesList(delegates),
		IncludeEmail: true,
	})
	if err != nil {
		return "", grpcutil.WrapIfTransient(err)
	}
	return resp.Token, nil
}

// delegatesList prepends `projects/-/serviceAccounts/` to emails.
func delegatesList(emails []string) []string {
	if len(emails) == 0 {
		return nil
	}
	out := make([]string, len(emails))
	for i, email := range emails {
		out[i] = "projects/-/serviceAccounts/" + email
	}
	return out
}

// networkAddrsForLog returns a string with IPv4 addresses of local network
// interfaces, if possible.
func networkAddrsForLog() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return fmt.Sprintf("failed to enumerate network interfaces: %s", err)
	}
	var ips []string
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipv4 := ipnet.IP.To4(); ipv4 != nil {
				ips = append(ips, ipv4.String())
			}
		}
	}
	if len(ips) == 0 {
		return "<no IPv4 interfaces>"
	}
	return strings.Join(ips, ", ")
}

// endUserIP extracts end-user IP address from X-Forwarded-For header.
func endUserIP(r auth.RequestMetadata) string {
	// X-Forwarded-For header is set by Cloud Load Balancer and GCP Serverless
	// load balancer and has format:
	//   [<untrusted part>,]<IP that connected to LB>,<unimportant>[,<more>].
	//
	// <untrusted part> may be present if the original request from the Internet
	// comes with X-Forwarded-For header. We can't trust IPs specified there. We
	// assume GCP load balancers sanitize the format of this field though.
	//
	// <IP that connected to LB> is what we are after.
	//
	// <unimportant> is "global forwarding rule external IP" for GKE or
	// the constant "169.254.1.1" for GCP Serverless. We don't care about these.
	//
	// <more> is present only if we proxy the request through more layers of
	// load balancers *while it is already inside GKE cluster*. We assume we don't
	// do that (if we ever do, Options{...} should be extended with a setting that
	// specifies how many layers of load balancers to skip to get to the original
	// IP). On GCP Serverless <more> is always empty.
	//
	// See https://cloud.google.com/load-balancing/docs/https for more info.
	forwardedFor := strings.Split(r.Header("X-Forwarded-For"), ",")
	if len(forwardedFor) >= 2 {
		return strings.TrimSpace(forwardedFor[len(forwardedFor)-2])
	}

	// Fallback to the peer IP if X-Forwarded-For is not set. Happens when
	// connecting to the server's port directly from within the cluster.
	ip, _, err := net.SplitHostPort(r.RemoteAddr())
	if err != nil {
		return "0.0.0.0"
	}
	return ip
}

// isHealthCheckerUA returns true for known user agents of health probers.
func isHealthCheckerUA(ua string) bool {
	switch {
	case strings.HasPrefix(ua, "kube-probe/"): // Kubernetes
		return true
	case strings.HasPrefix(ua, "GoogleHC"): // Cloud Load Balancer
		return true
	default:
		return false
	}
}

// resolveDependencies sorts modules based on their dependencies.
//
// Discovers unfulfilled required dependencies.
func resolveDependencies(mods []module.Module) ([]module.Module, error) {
	// Build a map: module.Name => module.Module
	modules := make(map[module.Name]module.Module, len(mods))
	for _, m := range mods {
		if _, ok := modules[m.Name()]; ok {
			return nil, errors.Reason("duplicate module %q", m.Name()).Err()
		}
		modules[m.Name()] = m
	}

	// Ensure all required dependencies exist, throw away missing optional
	// dependencies. The result is a directed graph that can be topo-sorted.
	graph := map[module.Name][]module.Name{}
	for _, m := range mods {
		for _, d := range m.Dependencies() {
			name := d.Dependency()
			if _, exists := modules[name]; !exists {
				if !d.Required() {
					continue
				}
				return nil, errors.Reason("module %q requires module %q which is not provided", m.Name(), name).Err()
			}
			graph[m.Name()] = append(graph[m.Name()], name)
		}
	}

	sorted := make([]module.Module, 0, len(graph))
	visited := make(map[module.Name]bool, len(graph))

	var visit func(n module.Name)
	visit = func(n module.Name) {
		if !visited[n] {
			visited[n] = true
			for _, dep := range graph[n] {
				visit(dep)
			}
			sorted = append(sorted, modules[n])
		}
	}

	for _, m := range mods {
		visit(m.Name())
	}
	return sorted, nil
}
