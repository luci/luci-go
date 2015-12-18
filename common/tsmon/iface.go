// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"flag"
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/tsmon/monitor"
	"github.com/luci/luci-go/common/tsmon/store"
	"github.com/luci/luci-go/common/tsmon/target"
	"golang.org/x/net/context"
)

var (
	// Store contains all the metric values for this process.  Applications
	// shouldn't need to access this directly - instead use the metric objects
	// which provide type-safe accessors.
	Store store.Store = &store.InMemoryStore{}

	// Monitor is the thing that sends metrics to monitoring endpoints.  Defaults
	// to a nil monitor, but changed by InitializeFromFlags.
	Monitor = monitor.NewNilMonitor()

	// Target contains information about this process, and is included in all
	// metrics reported by this process.
	Target target.Target
)

// InitializeFromFlags configures the tsmon library from flag values.
// This will set a Target (information about what's reporting metrics) and a
// Monitor (where to send the metrics to).
func InitializeFromFlags(c context.Context) error {
	logger := logging.Get(c)

	// Load the config file, and override its values with flags.
	config, err := loadConfig(*configFile)
	if err != nil {
		return err
	}

	if *endpoint != "" {
		config.Endpoint = *endpoint
	}
	if *credentials != "" {
		config.Credentials = *credentials
	}

	if config.Endpoint != "" {
		endpointURL, err := url.Parse(config.Endpoint)
		if err != nil {
			return err
		}

		switch endpointURL.Scheme {
		case "file":
			Monitor = monitor.NewDebugMonitor(logger)
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
	} else {
		logger.Warningf("Monitoring is disabled because no endpoint is configured")
	}

	t, err := target.NewFromFlags()
	if err != nil {
		return err
	}
	Target = t

	return nil
}

var (
	configFile = flag.String("ts-mon-config-file", defaultConfigFile(),
		"path to a JSON config file that contains suitable values for "+
			"\"endpoint\" and \"credentials\" for this machine. This config file is "+
			"intended to be shared by all processes on the machine, as the values "+
			"depend on the machine's position in the network, IP whitelisting and "+
			"deployment of credentials.")
	endpoint = flag.String("ts-mon-endpoint", "",
		"url (including file://, pubsub://project/topic) to post monitoring "+
			"metrics to. If set, overrides the value in --ts-mon-config-file")
	credentials = flag.String("ts-mon-credentials", "",
		"path to a pkcs8 json credential file. If set, overrides the value in "+
			"--ts-mon-config-file")
	flush = flag.String("ts-mon-flush", "manual",
		"metric push behavior: manual (only send when Flush() is called), or auto "+
			"(send automatically every --ts-mon-flush-interval)")
	flushInterval = flag.Duration("ts-mon-flush-interval", 60*time.Second,
		"automatically push metrics on this interval if --ts-mon-flush=auto")
)

func defaultConfigFile() string {
	if runtime.GOOS == "windows" {
		return "C:\\chrome-infra\\ts-mon.json"
	}
	return "/etc/chrome-infra/ts-mon.json"
}
