// Copyright 2023 The LUCI Authors.
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
	"fmt"
	"net/http"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/quota"
	"go.chromium.org/luci/server/quota/quotapb"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/router"
)

var quotaApp = quota.Register("example", &quota.ApplicationOptions{
	ResourceTypes: []string{"qps"},
})

func main() {
	modules := []module.Module{
		quota.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		ctx := srv.Context

		myRealm := "@internal:" + srv.Options.CloudProject
		polKey := &quotapb.PolicyKey{
			Namespace:    "global",
			Name:         "endpoint",
			ResourceType: "qps",
		}
		configID, err := quotaApp.LoadPoliciesAuto(srv.Context, myRealm, &quotapb.PolicyConfig{
			Policies: []*quotapb.PolicyConfig_Entry{
				{
					Key: polKey,
					Policy: &quotapb.Policy{
						Default: 10,
						Limit:   60,
						Refill: &quotapb.Policy_Refill{
							Units:    10,
							Interval: 12,
						},
					},
				},
			},
		})
		if err != nil {
			panic(err)
		}
		logging.Infof(ctx, "loaded policy: %s", configID)

		account := quotaApp.AccountID(myRealm, "global", "endpoint", "qps")
		policy := &quotapb.PolicyID{
			Config: configID,
			Key:    polKey,
		}

		// Set up a rate-limited endpoint by debiting 60 resources every time.
		// Returns an error if enough resources aren't available.
		srv.Routes.GET("/global-rate-limit-endpoint", nil, func(c *router.Context) {
			now := time.Now()
			resp, err := quota.ApplyOps(c.Request.Context(), "", nil, []*quotapb.Op{
				{
					AccountId:  account,
					PolicyId:   policy,
					RelativeTo: quotapb.Op_CURRENT_BALANCE,
					Delta:      -1,
				},
			})
			dur := time.Now().Sub(now)
			switch err {
			case nil:
				fmt.Fprintf(c.Writer, "OK (balance remaining: %d, %s ago, rpc time: %s)\n", resp.Results[0].NewBalance, time.Now().Sub(resp.OriginallySet.AsTime()), dur)
			case quota.ErrQuotaApply:
				c.Writer.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprintf(c.Writer, "Quota Denied (%s) (rpc time: %s)", resp, dur)
			default:
				errors.Log(c.Request.Context(), errors.Annotate(err, "debit quota").Err())
				http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
			}
		})

		// Set up a quota reset endpoint by restoring 60 resources every time.
		// The total resources cap at 60, so repeated calls are fine.
		srv.Routes.GET("/global-rate-limit-reset", nil, func(c *router.Context) {
			_, err := quota.ApplyOps(c.Request.Context(), "", nil, []*quotapb.Op{
				{
					AccountId:  account,
					PolicyId:   policy,
					RelativeTo: quotapb.Op_LIMIT,
				},
			})
			switch err {
			case nil:
				_, _ = c.Writer.Write([]byte("OK\n"))
			default:
				errors.Log(c.Request.Context(), errors.Annotate(err, "credit quota").Err())
				http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
			}
		})
		return nil
	})
}
