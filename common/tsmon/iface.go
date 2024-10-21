// Copyright 2015 The LUCI Authors.
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

package tsmon

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// Store returns the global metric store that contains all the metric values for
// this process.  Applications shouldn't need to access this directly - instead
// use the metric objects which provide type-safe accessors.
func Store(ctx context.Context) store.Store {
	return GetState(ctx).Store()
}

// InitializeFromFlags configures the tsmon library from flag values.
//
// This will set a Target (information about what's reporting metrics) and a
// Monitor (where to send the metrics to).
func InitializeFromFlags(ctx context.Context, fl *Flags) error {
	// Load the config file, and override its values with flags.
	cfg, err := loadConfig(fl.ConfigFile)
	if err != nil {
		return errors.Annotate(err, "failed to load config file at [%s]", fl.ConfigFile).Err()
	}

	if fl.Endpoint != "" {
		cfg.Endpoint = fl.Endpoint
	}
	if fl.Credentials != "" {
		cfg.Credentials = fl.Credentials
	}
	if fl.ActAs != "" {
		cfg.ActAs = fl.ActAs
	}

	mon, err := initMonitor(ctx, cfg)
	switch {
	case err != nil:
		return errors.Annotate(err, "failed to initialize monitor").Err()
	case mon == nil:
		return nil // tsmon is disabled
	}

	// Monitoring is enabled, so get the expensive default values for hostname,
	// etc.
	if cfg.AutoGenHostname {
		fl.Target.AutoGenHostname = true
	}
	if cfg.Hostname != "" {
		if fl.Target.DeviceHostname == "" {
			fl.Target.DeviceHostname = cfg.Hostname
		}
		if fl.Target.TaskHostname == "" {
			fl.Target.TaskHostname = cfg.Hostname
		}
	}
	if cfg.Region != "" {
		if fl.Target.DeviceRegion == "" {
			fl.Target.DeviceRegion = cfg.Region
		}
		if fl.Target.TaskRegion == "" {
			fl.Target.TaskRegion = cfg.Region
		}
	}
	fl.Target.SetDefaultsFromHostname()
	t, err := target.NewFromFlags(&fl.Target)
	if err != nil {
		return errors.Annotate(err, "failed to configure target from flags").Err()
	}

	state := GetState(ctx)
	state.SetMonitor(mon)
	state.SetStore(store.NewInMemory(t))

	if state.flusher != nil {
		logging.Infof(ctx, "Canceling previous tsmon auto flush")
		state.flusher.stop()
		state.flusher = nil
	}

	if fl.Flush == FlushAuto {
		state.flusher = &autoFlusher{}
		state.flusher.start(ctx, fl.FlushInterval)
	}

	return nil
}

// Shutdown gracefully terminates the tsmon by doing the final flush and
// disabling auto flush (if it was enabled).
//
// It resets Monitor and Store.
//
// Logs error to standard logger. Does nothing if tsmon wasn't initialized.
func Shutdown(ctx context.Context) {
	state := GetState(ctx)
	if store.IsNilStore(state.Store()) {
		return
	}

	if state.flusher != nil {
		logging.Debugf(ctx, "Stopping tsmon auto flush")
		state.flusher.stop()
		state.flusher = nil
	}

	// Flush the remaining metrics. This logs errors inside.
	_ = Flush(ctx)
	// We are done using this monitor.
	_ = state.monitor.Close()

	// Reset the state as if 'InitializeFromFlags' was never called.
	state.SetMonitor(monitor.NewNilMonitor())
	state.SetStore(store.NewNilStore())
}

// ResetCumulativeMetrics resets only cumulative metrics.
func ResetCumulativeMetrics(ctx context.Context) {
	GetState(ctx).ResetCumulativeMetrics(ctx)
}

// initMonitor examines flags and config and initializes a monitor.
//
// It returns (nil, nil) if tsmon should be disabled.
func initMonitor(ctx context.Context, cfg config) (monitor.Monitor, error) {
	if cfg.Endpoint == "" {
		logging.Infof(ctx, "tsmon is disabled because no endpoint is configured")
		return nil, nil
	}
	if strings.ToLower(cfg.Endpoint) == "none" {
		logging.Infof(ctx, "tsmon is explicitly disabled")
		return nil, nil
	}

	endpointURL, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, err
	}

	switch endpointURL.Scheme {
	case "file":
		return monitor.NewDebugMonitor(endpointURL.Path), nil
	case "https":
		// Ignore the path part of the URL for compatibility with legacy configs
		// that use "https://prodxmon-pa.googleapis.com/v1:insert". We only care
		// about the hostname for the gRPC monitor.
		hostname, port := endpointURL.Hostname(), endpointURL.Port()
		if port == "" {
			port = "443"
		}
		creds, err := newAuthenticator(ctx, cfg.Credentials, cfg.ActAs).PerRPCCredentials()
		if err != nil {
			return nil, err
		}
		conn, err := grpc.NewClient(
			fmt.Sprintf("%s:%s", hostname, port),
			grpc.WithTransportCredentials(credentials.NewTLS(nil)),
			grpc.WithPerRPCCredentials(creds),
		)
		if err != nil {
			return nil, errors.Annotate(err, "failed to dial ProdX service %s:%s", hostname, port).Err()
		}
		return monitor.NewGRPCMonitor(ctx, 500, conn), nil
	case "http":
		// We should not be sending credential tokens over an unencrypted channel.
		return nil, fmt.Errorf("unsupported tsmon endpoint url: %s", cfg.Endpoint)
	default:
		return nil, fmt.Errorf("unknown tsmon endpoint url: %s", cfg.Endpoint)
	}
}

// newAuthenticator returns a new authenticator for RPC requests.
func newAuthenticator(ctx context.Context, credentials, actAs string) *auth.Authenticator {
	authOpts := chromeinfra.DefaultAuthOptions()
	authOpts.ServiceAccountJSONPath = credentials
	authOpts.Scopes = []string{monitor.ProdXMonScope}
	authOpts.ActAsServiceAccount = actAs
	return auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts)
}
