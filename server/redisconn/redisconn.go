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

// Package redisconn implements integration with a Redis connection pool.
//
// Usage as a server module:
//
//	func main() {
//	  modules := []module.Module{
//	    redisconn.NewModuleFromFlags(),
//	  }
//	  server.Main(nil, modules, func(srv *server.Server) error {
//	    srv.Routes.GET("/", ..., func(c *router.Context) {
//	      conn, err := redisconn.Get(c.Context)
//	      if err != nil {
//	        // handle error
//	      }
//	      defer conn.Close()
//	      // use Redis API via `conn`
//	    })
//	    return nil
//	  })
//	}
//
// When used that way, Redis is also installed as the default implementation
// of caching.BlobCache (which basically speeds up various internal guts of
// the LUCI server framework).
//
// Can also be used as a low-level Redis connection pool library, see
// NewPool(...)
package redisconn

import (
	"context"
	"time"

	"github.com/gomodule/redigo/redis"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
)

// ErrNotConfigured is returned by Get if the context has no Redis pool inside.
var ErrNotConfigured = errors.New("Redis connection pool is not configured")

// Per-pool metrics derived from redis.Pool.Stats() by ReportStats.
var (
	connsMetric = metric.NewInt(
		"redis/pool/conns",
		"The number of connections in the pool (idle or in-use depending if state field)",
		&types.MetricMetadata{},
		field.String("pool"),  // e.g. "default"
		field.String("state"), // either "idle" or "in-use"
	)

	waitCountMetric = metric.NewCounter(
		"redis/pool/wait_count",
		"The total number of connections waited for.",
		&types.MetricMetadata{},
		field.String("pool"), // e.g. "default"
	)

	waitDurationMetric = metric.NewCounter(
		"redis/pool/wait_duration",
		"The total time blocked waiting for a new connection.",
		&types.MetricMetadata{Units: types.Microseconds},
		field.String("pool"), // e.g. "default"
	)
)

// NewPool returns a new pool configured with default parameters.
//
// "addr" is TCP "host:port" of a Redis server to connect to. No actual
// connection is established yet (this happens first time the pool is used).
//
// "db" is a index of a logical DB to SELECT in the connection by default,
// see https://redis.io/commands/select. It can be used as a weak form of
// namespacing. It is easy to bypass though, so please do not depend on it
// for anything critical (better to setup multiple Redis instances in this
// case).
//
// Doesn't use any authentication or encryption.
func NewPool(addr string, db int) *redis.Pool {
	// TODO(vadimsh): Tune the parameters or make them configurable. The values
	// below were picked somewhat arbitrarily.
	return &redis.Pool{
		MaxIdle:     64,
		MaxActive:   512,
		IdleTimeout: 3 * time.Minute,
		Wait:        true, // if all connections are busy, wait for an available one

		DialContext: func(ctx context.Context) (redis.Conn, error) {
			logging.Debugf(ctx, "Opening new Redis connection to %q...", addr)
			conn, err := redis.Dial("tcp", addr,
				redis.DialDatabase(db),
				redis.DialConnectTimeout(5*time.Second),
				redis.DialReadTimeout(5*time.Second),
				redis.DialWriteTimeout(5*time.Second),
			)
			if err != nil {
				return nil, errors.Fmt("redis: %w", err)
			}
			return conn, nil
		},

		// If the connection was idle for more than a minute, verify it is still
		// alive by pinging the server.
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}
}

var contextKey = "redisconn.Pool"

// UsePool installs a connection pool into the context, to be used by Get.
func UsePool(ctx context.Context, pool *redis.Pool) context.Context {
	return context.WithValue(ctx, &contextKey, pool)
}

// GetPool returns a connection pool in the context or nil if not there.
func GetPool(ctx context.Context) *redis.Pool {
	p, _ := ctx.Value(&contextKey).(*redis.Pool)
	return p
}

// ReportStats reports the connection pool stats as tsmon metrics.
//
// For best results should be called once a minute or right before tsmon flush.
//
// "name" is used as "pool" metric field, to distinguish pools between each
// other.
func ReportStats(ctx context.Context, pool *redis.Pool, name string) {
	stats := pool.Stats()
	connsMetric.Set(ctx, int64(stats.IdleCount), name, "idle")
	connsMetric.Set(ctx, int64(stats.ActiveCount-stats.IdleCount), name, "in-use")
	waitCountMetric.Set(ctx, int64(stats.WaitCount), name)
	waitDurationMetric.Set(ctx, int64(stats.WaitDuration.Nanoseconds()/1000), name)
}

// Get returns a Redis connection using the pool installed in the context.
//
// May block until such connection is available. Returns an error if the
// context expires before that. The returned connection itself is not associated
// with the context and can outlive it.
//
// The connection MUST be explicitly closed as soon as it's no longer needed,
// otherwise leaks and slow downs are eminent.
func Get(ctx context.Context) (redis.Conn, error) {
	if p := GetPool(ctx); p != nil {
		return p.GetContext(ctx)
	}
	return nil, ErrNotConfigured
}
