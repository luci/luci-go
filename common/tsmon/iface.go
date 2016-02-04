// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/gcloud/pubsub"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/tsmon/monitor"
	"github.com/luci/luci-go/common/tsmon/store"
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/common/tsmon/types"
)

var (
	globalTarget  types.Target
	globalStore   = store.NewNilStore()
	globalMonitor = monitor.NewNilMonitor()
	globalFlusher *autoFlusher

	registeredMetrics     = map[string]types.Metric{}
	registeredMetricsLock sync.RWMutex
)

// Target contains information about this process, and is included in all
// metrics reported by this process.
func Target() types.Target {
	return globalTarget
}

// Store returns the global metric store that contains all the metric values for
// this process.  Applications shouldn't need to access this directly - instead
// use the metric objects which provide type-safe accessors.
func Store() store.Store {
	return globalStore
}

// Monitor returns the global monitor that sends metrics to monitoring
// endpoints.  Defaults to a nil monitor, but changed by InitializeFromFlags.
func Monitor() monitor.Monitor {
	return globalMonitor
}

// Register is called by metric objects to register themselves.  This will panic
// if another metric with the same name is already registered.
func Register(m types.Metric) {
	registeredMetricsLock.Lock()
	defer registeredMetricsLock.Unlock()

	if _, ok := registeredMetrics[m.Info().Name]; ok {
		panic(fmt.Sprintf("A metric with the name '%s' was already registered", m.Info().Name))
	}

	registeredMetrics[m.Info().Name] = m

	if globalStore != nil {
		globalStore.Register(m)
	}
}

// Unregister is called by metric objects to unregister themselves.
func Unregister(m types.Metric) {
	registeredMetricsLock.Lock()
	defer registeredMetricsLock.Unlock()

	delete(registeredMetrics, m.Info().Name)

	if globalStore != nil {
		globalStore.Unregister(m)
	}
}

// SetStore changes the global metric store.  All metrics that were registered
// with the old store will be re-registered on the new store.
func SetStore(s store.Store) {
	if s == globalStore {
		return
	}

	registeredMetricsLock.RLock()
	defer registeredMetricsLock.RUnlock()

	// Register metrics on the new store.
	for _, m := range registeredMetrics {
		s.Register(m)
	}

	oldStore := globalStore
	globalStore = s

	// Unregister metrics from the old store.
	if oldStore != nil {
		for _, m := range registeredMetrics {
			globalStore.Unregister(m)
		}
	}
}

// InitializeFromFlags configures the tsmon library from flag values.
//
// This will set a Target (information about what's reporting metrics) and a
// Monitor (where to send the metrics to).
func InitializeFromFlags(c context.Context, fl *Flags) error {
	mon, err := initMonitor(c, fl)
	switch {
	case err != nil:
		return err
	case mon == nil:
		return nil // tsmon is disabled
	}

	// Monitoring is enabled, so get the expensive default values for hostname,
	// etc.
	fl.Target.SetDefaultsFromHostname()
	t, err := target.NewFromFlags(&fl.Target)
	if err != nil {
		return err
	}

	globalMonitor = mon
	globalTarget = t
	SetStore(store.NewInMemory())

	if globalFlusher != nil {
		logging.Infof(c, "Canceling previous tsmon auto flush")
		globalFlusher.stop()
		globalFlusher = nil
	}

	if fl.Flush == "auto" {
		globalFlusher = &autoFlusher{}
		globalFlusher.start(c, fl.FlushInterval)
	}

	return nil
}

// Shutdown gracefully terminates the tsmon by doing the final flush and
// disabling auto flush (if it was enabled).
//
// It resets Target, Monitor and Store.
//
// Logs error to standard logger. Does nothing if tsmon wasn't initialized.
func Shutdown(c context.Context) {
	if store.IsNilStore(globalStore) {
		return
	}

	if globalFlusher != nil {
		logging.Debugf(c, "Stopping tsmon auto flush")
		globalFlusher.stop()
		globalFlusher = nil
	}

	logging.Debugf(c, "Doing the final tsmon flush")
	if err := Flush(c); err != nil {
		logging.Errorf(c, "Final tsmon flush failed - %s", err)
	} else {
		logging.Debugf(c, "Final tsmon flush finished")
	}

	// Reset the state as if 'InitializeFromFlags' was never called.
	globalMonitor = monitor.NewNilMonitor()
	globalTarget = nil
	SetStore(store.NewNilStore())
}

// initMonitor examines flags and config and initializes a monitor.
//
// It returns (nil, nil) if tsmon should be disabled.
func initMonitor(c context.Context, fl *Flags) (monitor.Monitor, error) {
	// Load the config file, and override its values with flags.
	config, err := loadConfig(fl.ConfigFile)
	if err != nil {
		logging.Warningf(c, "tsmon is disabled because the config file (%s) could not be loaded: %s",
			fl.ConfigFile, err)
		return nil, nil
	}

	if fl.Endpoint != "" {
		config.Endpoint = fl.Endpoint
	}
	if fl.Credentials != "" {
		config.Credentials = fl.Credentials
	}

	if config.Endpoint == "" {
		logging.Warningf(c, "tsmon is disabled because no endpoint is configured")
		return nil, nil
	}

	endpointURL, err := url.Parse(config.Endpoint)
	if err != nil {
		return nil, err
	}

	switch endpointURL.Scheme {
	case "file":
		return monitor.NewDebugMonitor(logging.Get(c), endpointURL.Path), nil
	case "pubsub":
		cl, err := makeClient(c, config.Credentials)
		if err != nil {
			return nil, err
		}
		return monitor.NewPubsubMonitor(c, cl, endpointURL.Host, strings.TrimPrefix(endpointURL.Path, "/"))
	default:
		return nil, fmt.Errorf("unknown tsmon endpoint url: %s", config.Endpoint)
	}
}

// makeClient returns http.Client that knows how to send authenticated requests
// to PubSub API.
func makeClient(c context.Context, credentials string) (*http.Client, error) {
	authOpts := auth.Options{
		Context: c,
		Scopes:  pubsub.PublisherScopes,
	}
	if credentials == GCECredentials {
		authOpts.Method = auth.GCEMetadataMethod
	} else {
		authOpts.Method = auth.ServiceAccountMethod
		authOpts.ServiceAccountJSONPath = credentials
	}
	return auth.NewAuthenticator(auth.SilentLogin, authOpts).Client()
}
