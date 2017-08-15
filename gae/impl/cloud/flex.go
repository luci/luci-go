// Copyright 2017 The LUCI Authors.
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

package cloud

import (
	"net/http"
	"os"
	"strings"

	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/errors"

	"cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/datastore"
	cloudLogging "cloud.google.com/go/logging"
	iamAPI "google.golang.org/api/iam/v1"
	"google.golang.org/api/option"

	"golang.org/x/net/context"
)

// Flex defines a Google AppEngine Flex Environment platform.
type Flex struct {
	// Cache is the process-global LRU cache instance that Flexi services can use
	// to cache data.
	//
	// If Cache is nil, a default cache will be used.
	Cache *lru.Cache
}

// Configure constructs a Config based on the current Flex environment.
//
// Configure will instantiate some cloud clients. It is the responsibility of
// the client to close those instances when finished.
//
// opts is the optional set of client options to pass to cloud platform clients
// that are instantiated.
func (f *Flex) Configure(c context.Context, opts ...option.ClientOption) (cfg *Config, err error) {
	// If we aren't in an AppEngine Flex environment, we will panic further in the
	// code with an "error" type.
	defer func() {
		if r := recover(); r != nil {
			err = errors.Annotate(r.(error), "not in an AppEngine Flex environment").Err()
		}
	}()

	mustGetEnv := func(key string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		panic(errors.Reason("missing environment variable %q", key).Err())
	}
	mustGetMetadata := func(key string) string {
		switch v, err := metadata.Get(key); {
		case err != nil:
			panic(errors.Annotate(err, "could not retrieve metadata value %q", key).Err())
		case v == "":
			panic(errors.Reason("missing metadata value %q", key).Err())
		default:
			return v
		}
	}

	// Probe environment for values.
	cfg = &Config{
		ProjectID:          mustGetEnv("GCLOUD_PROJECT"),
		ServiceName:        mustGetEnv("GAE_SERVICE"),
		VersionName:        mustGetEnv("GAE_VERSION"),
		InstanceID:         mustGetEnv("GAE_INSTANCE"),
		ServiceAccountName: mustGetMetadata("/instance/service-accounts/default/email"),
	}

	gsp := GoogleServiceProvider{
		ServiceAccount: cfg.ServiceAccountName,
		Cache:          f.Cache,
	}
	if gsp.Cache == nil {
		gsp.Cache = lru.New(defaultGoogleServicesCacheSize)
	}
	cfg.ServiceProvider = &gsp

	// Augment our client options. First we clone them so we don't mutate our
	// caller's option set.
	ts, err := gsp.TokenSource(c, iamAPI.CloudPlatformScope)
	if err != nil {
		return nil, err
	}

	opts = append(make([]option.ClientOption, 0, len(opts)+1), opts...)
	opts = append(opts, option.WithTokenSource(ts))

	// Cloud Datastore Client
	if cfg.DS, err = datastore.NewClient(c, cfg.ProjectID, opts...); err != nil {
		panic(errors.Annotate(err, "failed to instantiate datastore client").Err())
	}

	// Cloud Logging Client
	if cfg.L, err = cloudLogging.NewClient(c, cfg.ProjectID, opts...); err != nil {
		panic(errors.Annotate(err, "could not create logger").Err())
	}

	return
}

// Request probes Request parameters from a AppEngine Flex Environment HTTP
// request.
func (*Flex) Request(req *http.Request) *Request {
	return &Request{
		TraceID: getCloudTraceContext(req),
	}
}

// getCloudTraceContext parses an "X-Cloud-Trace-Context" header.
//
// The "X-Cloud-Trace-Context" header is supplied with requests, and is
// formatted: "TRACE-ID/SPAN_ID;o=TRACE_TRUE".
//
// The "TRACE-ID" should be a unique 32-character hex value representing an
// ID that is unique between requests, so we will use that as a base for
// per-request uniqueness.
//
// https://cloud.google.com/trace/docs/faq
func getCloudTraceContext(req *http.Request) string {
	v := req.Header.Get("X-Cloud-Trace-Context")
	if v == "" {
		return ""
	}
	return strings.SplitN(v, "/", 2)[0]
}
