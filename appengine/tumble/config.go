// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tumble

import (
	"fmt"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"golang.org/x/net/context"
)

// Config is the set of tweakable things for tumble. If you use something other
// than the defaults (e.g. unset values), you must ensure that all aspects of
// your application use the same config.
type Config struct {
	// Name is the name of this service. This is the expected name of the
	// configured taskqueue, as well as the prefix used for things like memcache
	// keys.
	//
	// It defaults to "tumble". It is illegal for the Name to contain '/', and
	// Use will panic if it does.
	Name string

	// URLPrefix is the prefix to append for all registered routes. It's
	// normalized to begin and end with a '/'. So "wat" would register:
	//    "/wat/{Service.Name}/fire_all_tasks"
	//    "/wat/{Service.Name}/process_shard/:shard_id/at/:timestamp"
	//
	// This defaults to "internal"
	URLPrefix string

	// NumShards is the number of tumble shards that will process concurrently.
	// It defaults to 32.
	NumShards uint64

	// TemporalMinDelay is the minimum number of seconds to wait before the
	// task queue entry for a given shard will run. It defaults to 1 second.
	TemporalMinDelay time.Duration

	// TemporalRoundFactor is the number of seconds to batch together in task
	// queue tasks. It defaults to 4 seconds.
	TemporalRoundFactor time.Duration

	// NumGoroutines is the number of gorountines that will process in parallel
	// in a single shard. Each goroutine will process exactly one root entity.
	// It defaults to 16.
	NumGoroutines uint64

	// ProcessMaxBatchSize is the number of mutations that each processor goroutine
	// will attempt to include in each commit.
	//
	// It defaults to 128. A negative value means no limit.
	ProcessMaxBatchSize int32
}

type key int

var defaultConfig = Config{
	Name:                "tumble",
	URLPrefix:           "/internal/",
	NumShards:           32,
	TemporalMinDelay:    time.Second,
	TemporalRoundFactor: 4 * time.Second,
	NumGoroutines:       16,
	ProcessMaxBatchSize: 128,
}

// DefaultConfig returns a Config with all the default values populated.
func DefaultConfig() Config {
	return defaultConfig
}

// Use allows you to set a specific configuration in the context. This
// configuration can be obtained by calling GetConfig. Any zero-value fields
// in the Config will be replaced with its default value.
//
// This Config may be retrieved with GetConfig.
func Use(c context.Context, cfg Config) context.Context {
	if cfg.Name == "" {
		cfg.Name = defaultConfig.Name
	}
	if strings.Contains(cfg.Name, "/") {
		panic(fmt.Errorf("tumble: name may not contain '/': %q", cfg.Name))
	}
	if cfg.URLPrefix == "" {
		cfg.URLPrefix = defaultConfig.URLPrefix
	}
	if !strings.HasPrefix(cfg.URLPrefix, "/") {
		cfg.URLPrefix = "/" + cfg.URLPrefix
	}
	if !strings.HasSuffix(cfg.URLPrefix, "/") {
		cfg.URLPrefix = cfg.URLPrefix + "/"
	}
	if cfg.NumShards == 0 {
		cfg.NumShards = defaultConfig.NumShards
	}
	if cfg.TemporalMinDelay == 0 {
		cfg.TemporalMinDelay = defaultConfig.TemporalMinDelay
	}
	if cfg.TemporalRoundFactor == 0 {
		cfg.TemporalRoundFactor = defaultConfig.TemporalRoundFactor
	}
	if cfg.NumGoroutines == 0 {
		cfg.NumGoroutines = defaultConfig.NumGoroutines
	}
	if cfg.ProcessMaxBatchSize == 0 {
		cfg.ProcessMaxBatchSize = defaultConfig.ProcessMaxBatchSize
	}
	return context.WithValue(c, key(0), &cfg)
}

// GetConfig retrieves the Config from the current context. If none has been set,
// this returns a Config which has all the defaults filled out.
func GetConfig(c context.Context) Config {
	if cfg, ok := c.Value(key(0)).(*Config); ok {
		return *cfg
	}
	return defaultConfig
}

const processShardURLFormat = "/process_shard/:shard_id/at/:timestamp"

// ProcessURLPattern returns the httprouter-style URL pattern for the taskqueue
// process handler.
func (c *Config) ProcessURLPattern() string {
	return c.URLPrefix + c.Name + processShardURLFormat
}

// ProcessURL creates a new url for a process shard taskqueue task, including
// the given timestamp and shard number.
func (c *Config) ProcessURL(ts time.Time, shard uint64) string {
	return strings.NewReplacer(
		":shard_id", fmt.Sprint(shard),
		":timestamp", fmt.Sprint(ts.Unix())).Replace(c.ProcessURLPattern())
}

// FireAllTasksURL returns the url intended to be hit by appengine cron to fire
// an instance of all the processing tasks.
func (c *Config) FireAllTasksURL() string {
	return c.URLPrefix + c.Name + "/fire_all_tasks"
}

// InstallHandlers installs http handlers
func (c *Config) InstallHandlers(r *httprouter.Router) {
	// GET so that this can be invoked from cron
	r.GET(c.FireAllTasksURL(),
		gaemiddleware.BaseProd(gaemiddleware.RequireCron(FireAllTasksHandler)))

	r.POST(c.ProcessURLPattern(),
		gaemiddleware.BaseProd(gaemiddleware.RequireTaskQueue(c.Name, ProcessShardHandler)))
}
