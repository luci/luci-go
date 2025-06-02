// Copyright 2022 The LUCI Authors.
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

// Package main contains a binary demonstrating how to use the server/quota
// module to implement rate limiting for requests.
package main

import (
	"net/http"

	"github.com/alicebob/miniredis/v2"
	"github.com/gomodule/redigo/redis"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/module"
	quota "go.chromium.org/luci/server/quotabeta"
	pb "go.chromium.org/luci/server/quotabeta/proto"
	"go.chromium.org/luci/server/quotabeta/quotaconfig"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/router"
)

func main() {
	modules := []module.Module{
		quota.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
	}

	// Configure an in-memory redis database.
	s, err := miniredis.Run()
	if err != nil {
		panic(err)
	}
	defer s.Close()

	server.Main(nil, modules, func(srv *server.Server) error {
		// Initialize a static, in-memory implementation of quotaconfig.Interface.
		m, err := quotaconfig.NewMemory(srv.Context, []*pb.Policy{
			// Policy governing a global rate limit of one request per minute to the
			// /global-rate-limit-endpoint handler. 60 resources are available and
			// the handler consumes 60 resources every time it's called (see below),
			// while the policy is configured to automatically replenish one resource
			// every second. This quota can be reset by sending a request to the
			// /global-rate-limit-reset handler. 60 resources are replenished every time
			// it's called (see below), and the default 60 resources also functions as a
			// cap.
			{
				Name:          "global-rate-limit",
				Resources:     60,
				Replenishment: 1,
			},
		})
		if err != nil {
			panic(err)
		}

		// Register the quotaconfig.Interface and &redis.Pool.
		srv.Context = redisconn.UsePool(quota.Use(srv.Context, m), &redis.Pool{
			Dial: func() (redis.Conn, error) {
				return redis.Dial("tcp", s.Addr())
			},
		})

		// Set up a rate-limited endpoint by debiting 60 resources every time.
		// Returns an error if enough resources aren't available.
		srv.Routes.GET("/global-rate-limit-endpoint", nil, func(c *router.Context) {
			updates := map[string]int64{
				"global-rate-limit": -60,
			}
			switch err := quota.UpdateQuota(c.Request.Context(), updates, nil); err {
			case nil:
				_, _ = c.Writer.Write([]byte("OK\n"))
			case quota.ErrInsufficientQuota:
				http.Error(c.Writer, "rate limit exceeded", http.StatusTooManyRequests)
			default:
				errors.Log(c.Request.Context(), errors.Fmt("debit quota: %w", err))
				http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
			}
		})

		// Set up a quota reset endpoint by restoring 60 resources every time.
		// The total resources cap at 60, so repeated calls are fine.
		srv.Routes.GET("/global-rate-limit-reset", nil, func(c *router.Context) {
			updates := map[string]int64{
				"global-rate-limit": 60,
			}
			switch err := quota.UpdateQuota(c.Request.Context(), updates, nil); err {
			case nil:
				_, _ = c.Writer.Write([]byte("OK\n"))
			default:
				errors.Log(c.Request.Context(), errors.Fmt("credit quota: %w", err))
				http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
			}
		})
		return nil
	})
}
