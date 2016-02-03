// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/tsmon/monitor"
	"github.com/luci/luci-go/common/tsmon/store"
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"
)

var (
	// Target contains information about this process, and is included in all
	// metrics reported by this process.
	Target types.Target

	globalStore   = store.NewInMemory()
	globalMonitor = monitor.NewNilMonitor()

	registeredMetrics     = map[string]types.Metric{}
	registeredMetricsLock sync.RWMutex

	cancelAutoFlush context.CancelFunc
)

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
// This will set a Target (information about what's reporting metrics) and a
// Monitor (where to send the metrics to).
func InitializeFromFlags(c context.Context, fl *Flags) error {
	logger := logging.Get(c)

	if err := initEndpoint(c, fl, logger); err != nil {
		return err
	}

	t, err := target.NewFromFlags(&fl.Target)
	if err != nil {
		return err
	}
	Target = t

	SetStore(store.NewInMemory())

	if cancelAutoFlush != nil {
		logger.Infof("Cancelling previous tsmon auto flush")
		cancelAutoFlush()
		cancelAutoFlush = nil
	}

	if fl.Flush == "auto" {
		var flushCtx context.Context
		flushCtx, cancelAutoFlush = context.WithCancel(c)
		go autoFlush(flushCtx, fl.FlushInterval)
	}

	return nil
}

func initEndpoint(c context.Context, fl *Flags, logger logging.Logger) error {
	// Load the config file, and override its values with flags.
	config, err := loadConfig(fl.ConfigFile)
	if err != nil {
		logger.Warningf("Monitoring is disabled because the config file (%s) could not be loaded: %s",
			fl.ConfigFile, err)
		return nil
	}

	if fl.Endpoint != "" {
		config.Endpoint = fl.Endpoint
	}
	if fl.Credentials != "" {
		config.Credentials = fl.Credentials
	}

	if config.Endpoint == "" {
		logger.Warningf("Monitoring is disabled because no endpoint is configured")
		return nil
	}

	endpointURL, err := url.Parse(config.Endpoint)
	if err != nil {
		return err
	}

	switch endpointURL.Scheme {
	case "file":
		globalMonitor = monitor.NewDebugMonitor(logger, endpointURL.Path)
	case "pubsub":
		m, err := monitor.NewPubsubMonitor(
			config.Credentials, endpointURL.Host, strings.TrimPrefix(endpointURL.Path, "/"))
		if err != nil {
			return err
		}
		globalMonitor = m
	default:
		return fmt.Errorf("unknown tsmon endpoint url: %s", config.Endpoint)
	}

	// Monitoring is enabled, so get the expensive default values for hostname,
	// etc.
	fl.Target.SetDefaultsFromHostname()
	return nil
}
