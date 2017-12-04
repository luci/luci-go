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

package cloud

import (
	"net/http"
	"time"

	"go.chromium.org/gae/impl/dummy"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/mail"
	mc "go.chromium.org/gae/service/memcache"
	"go.chromium.org/gae/service/module"
	"go.chromium.org/gae/service/taskqueue"
	"go.chromium.org/gae/service/user"

	"go.chromium.org/luci/common/clock"

	"cloud.google.com/go/datastore"
	cloudLogging "cloud.google.com/go/logging"
	"github.com/bradfitz/gomemcache/memcache"

	"golang.org/x/net/context"
)

var requestStateContextKey = "gae/flex request state"

// Config is a full-stack cloud service configuration. A user can selectively
// populate its fields, and services for the populated fields will be installed
// in the Context and available.
//
// Because the "impl/cloud" service collection is a composite set of cloud
// services, the user can choose services based on their configuration.
//
// The parameters of Config are mostly consumed by the "service/info" service
// implementation, which describes the environment in which the service is run.
type Config struct {
	// IsDev is true if this is a development execution.
	IsDev bool

	// ProjectID, if not empty, is the project ID returned by the "info" service.
	//
	// If empty, the service will treat requests for this field as not
	// implemented.
	ProjectID string

	// ServiceName, if not empty, is the service (module) name returned by the
	// "info" service.
	//
	// If empty, the service will treat requests for this field as not
	// implemented.
	ServiceName string

	// VersionName, if not empty, is the version name returned by the "info"
	// service.
	//
	// If empty, the service will treat requests for this field as not
	// implemented.
	VersionName string

	// InstanceID, if not empty, is the instance ID returned by the "info"
	// service.
	//
	// If empty, the service will treat requests for this field as not
	// implemented.
	InstanceID string

	// ServiceAccountName, if not empty, is the service account name returned by
	// the "info" service.
	//
	// If empty, the service will treat requests for this field as not
	// implemented.
	ServiceAccountName string

	// ServiceProvider, if not nil, is the system service provider to use for
	// non-cloud external resources and services.
	//
	// If nil, the service will treat requests for services as not implemented.
	ServiceProvider ServiceProvider

	// LogToSTDERR, if true, indicates that log messages should be tee'd to
	// a simple STDERR logger prior to being written.
	//
	// This is be useful because Flex environment has a separate processing
	// pipeline for logs written to STDOUT/STDERR, so this ensures that even if
	// something goes wrong with the Cloud Logging client, the log may still be
	// recorded.
	LogToSTDERR bool

	// DS is the cloud datastore client. If populated, the datastore service will
	// be installed.
	DS *datastore.Client

	// MC is the memcache service client. If populated, the memcache service will
	// be installed.
	MC *memcache.Client

	// RequestLogger, if not nil, will be used by ScopedRequest to log
	// request-level logs.
	//
	// The request log is a per-request high-level log that shares a Trace ID
	// with individual debug logs, has request-wide metadata, and is given the
	// severity of the highest debug log emitted during the handling of the
	// request.
	RequestLogger *cloudLogging.Logger

	// DebugLogger, if not nil, will cause a Stackdriver Logging client to be
	// installed into the Context by Use for debug logging messages.
	//
	// Debug logging messages are individual application log messages emitted
	// through the "logging.Logger" interface installed by Use. All logs emitted
	// through the Logger are considered debug logs, regardless of theirl
	// individual Level.
	DebugLogger *cloudLogging.Logger
}

// Request is the set of request-specific parameters.
type Request struct {
	// TraceID, if not empty, is the request's trace ID returned by the "info"
	// service.
	//
	// If empty, the service will treat requests for this field as not
	// implemented.
	TraceID string

	// HTTPRequest, if not nil, is the HTTP request that is being handled.
	HTTPRequest *http.Request

	// StartTime is the time when this request started. If empty, the current
	// clock time at the point when Use is called will be recorded.
	StartTime time.Time

	// LocalAddr is the local address handling this request. It may be empty
	// if the local address is unknown.
	LocalAddr string

	// SeverityTracker tracks the severity of the overall request. Callers may use
	// this manually. If not nil, it will be supplied to the request's Logger.
	//
	// If nil, the highest logging severity will still be tracked.
	SeverityTracker *LogSeverityTracker
}

// requestState is installed into the Context by Use to track the state of the
// current Request.
type requestState struct {
	Request

	cfg               *Config
	insertIDGenerator *InsertIDGenerator
}

// currentRequestState returns the requestState in c. If no requestState is
// installed, currentRequestState will return nil.
func currentRequestState(c context.Context) *requestState {
	rs, _ := c.Value(&requestStateContextKey).(*requestState)
	return rs
}

func (rs *requestState) with(c context.Context) context.Context {
	return context.WithValue(c, &requestStateContextKey, rs)
}

// Use installs the Config into the supplied Context. Services will be installed
// based on the fields that are populated in Config.
//
// req is optional. If not nil, its fields will be used to initialize the
// services installed into the Context.
//
// Any services that are missing will have "impl/dummy" stubs installed. These
// stubs will panic if called.
func (cfg *Config) Use(c context.Context, req *Request) context.Context {
	if req == nil {
		req = &Request{}
	}

	// Fill our missing Request fields and install it into c.
	rs := requestState{
		Request: *req,
		cfg:     cfg,
	}
	if rs.StartTime.IsZero() {
		rs.StartTime = clock.Now(c)
	}
	if rs.SeverityTracker == nil {
		rs.SeverityTracker = &LogSeverityTracker{}
	}
	if rs.TraceID != "" {
		rs.insertIDGenerator = &InsertIDGenerator{Base: rs.TraceID}
	}
	c = rs.with(c)

	// Dummy services that we don't support.
	c = mail.Set(c, dummy.Mail())
	c = module.Set(c, dummy.Module())
	c = taskqueue.SetRaw(c, dummy.TaskQueue())
	c = user.Set(c, dummy.User())

	// Install the logging service, if fields are sufficiently configured.
	// If no logging service is available, fall back onto an existing logger in
	// the context (usually a console (STDERR) logger).
	//
	// trace_id label is required to magically associate the logs produced by us
	// with GAE own request logs. This also assumes Resource fields in the logger
	// are already properly set to indicate "gae_app" resource. See:
	//
	//	https://github.com/GoogleCloudPlatform/google-cloud-go/issues/720
	if cfg.DebugLogger != nil {
		lcfg := LoggerConfig{
			SeverityTracker:   rs.SeverityTracker,
			InsertIDGenerator: rs.insertIDGenerator,
			Trace:             rs.TraceID,
			LogToSTDERR:       cfg.LogToSTDERR,
			Labels:            map[string]string{},
		}
		if req.TraceID != "" {
			lcfg.Labels[TraceIDLogLabel] = req.TraceID
		}
		c = WithLogger(c, cfg.DebugLogger, &lcfg)
	}

	// Setup and install the "info" service.
	gi := serviceInstanceGlobalInfo{
		Config:  cfg,
		Request: req,
	}
	c = useInfo(c, &gi)

	// datastore service
	if cfg.DS != nil {
		cds := cloudDatastore{
			client: cfg.DS,
		}
		c = cds.use(c)
	} else {
		c = ds.SetRaw(c, dummy.Datastore())
	}

	// memcache service
	if cfg.MC != nil {
		mc := memcacheClient{
			client: cfg.MC,
		}
		c = mc.use(c)
	} else {
		c = mc.SetRaw(c, dummy.Memcache())
	}

	return c
}

// HTTPRequest returns the http.Request object associated with the current
// Flex request Context.
//
// c must be a Context supplied by Use. If the source Request passed to Use did
// not have an HTTP request associated with it, HTTPRequest will return
// nil.
func HTTPRequest(c context.Context) *http.Request {
	return currentRequestState(c).HTTPRequest
}
