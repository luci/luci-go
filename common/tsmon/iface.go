// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/tsmon/monitor"
	"github.com/luci/luci-go/common/tsmon/store"
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"
)

var (
	// Store contains all the metric values for this process.  Applications
	// shouldn't need to access this directly - instead use the metric objects
	// which provide type-safe accessors.
	Store = store.NewInMemory()

	// Monitor is the thing that sends metrics to monitoring endpoints.  Defaults
	// to a nil monitor, but changed by InitializeFromFlags.
	Monitor = monitor.NewNilMonitor()

	// Target contains information about this process, and is included in all
	// metrics reported by this process.
	Target types.Target

	cancelAutoFlush context.CancelFunc
)

// InitializeFromFlags configures the tsmon library from flag values.
// This will set a Target (information about what's reporting metrics) and a
// Monitor (where to send the metrics to).
func InitializeFromFlags(c context.Context, fl *Flags) error {
	logger := logging.Get(c)

	// Load the config file, and override its values with flags.
	config, err := loadConfig(fl.ConfigFile)
	if err != nil {
		return err
	}

	if fl.Endpoint != "" {
		config.Endpoint = fl.Endpoint
	}
	if fl.Credentials != "" {
		config.Credentials = fl.Credentials
	}

	if config.Endpoint != "" {
		endpointURL, err := url.Parse(config.Endpoint)
		if err != nil {
			return err
		}

		switch endpointURL.Scheme {
		case "file":
			Monitor = monitor.NewDebugMonitor(logger, endpointURL.Path)
		case "pubsub":
			m, err := monitor.NewPubsubMonitor(
				config.Credentials, endpointURL.Host, strings.TrimPrefix(endpointURL.Path, "/"))
			if err != nil {
				return err
			}
			Monitor = m
		default:
			return fmt.Errorf("unknown tsmon endpoint url: %s", config.Endpoint)
		}

		// Monitoring is enabled, so get the expensive default values for hostname,
		// etc.
		fl.Target.SetDefaultsFromHostname()
	} else {
		logger.Warningf("Monitoring is disabled because no endpoint is configured")
	}

	t, err := target.NewFromFlags(&fl.Target)
	if err != nil {
		return err
	}
	Target = t

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
