// Copyright 2018 The LUCI Authors.
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

package buildbucket

import (
	"fmt"
	"net/http"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	swarmbucket "go.chromium.org/luci/common/api/buildbucket/swarmbucket/v1"
	sv1 "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/milo/buildsource/swarming"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/server/auth"
)

// parsePools parses out all of the pools in the builder response.
func parseBuilders(c context.Context, r *swarmbucket.SwarmingSwarmbucketApiGetBuildersResponseMessage) []model.BuilderPool {
	result := []model.BuilderPool{}
	now := clock.Now(c)
	for _, bucket := range r.Buckets {
		for _, builder := range bucket.Builders {
			id := fmt.Sprintf("buildbucket/%s/%s", bucket.Name, builder.Name)
			pool := model.BuilderPool{
				BuilderKey: datastore.MakeKey(c, "BuilderSummary", id),
				Dimensions: model.NewDimensions(bucket.SwarmingHostname, builder.SwarmingDimensions),
				LastUpdate: now,
			}
			result = append(result, pool)
		}
	}
	return result
}

// uniqueDimensions returns all unique set of dimensions in the pool.
func uniqueDimensions(p []model.BuilderPool) []model.Dimensions {
	// Key: Dimension SHA1
	set := map[string]model.Dimensions{}
	for _, pool := range p {
		if _, ok := set[pool.Dimensions.SHA1]; !ok {
			set[pool.Dimensions.SHA1] = pool.Dimensions
		}
	}
	result := make([]model.Dimensions, 0, len(set))
	for _, dim := range set {
		result = append(result, dim)
	}
	return result
}

func botStatus(c context.Context, bot *sv1.SwarmingRpcsBotInfo) (*model.Machine, error) {
	lastSeen, err := time.Parse(swarming.SwarmingTimeLayout, bot.LastSeenTs)
	if err != nil {
		return nil, err
	}
	result := &model.Machine{
		Name:     bot.BotId,
		LastSeen: lastSeen,
	}

	switch {
	case bot.TaskId != "":
		result.Status = model.Busy
	case bot.IsDead && clock.Now(c).Sub(lastSeen) < 5*time.Minute:
		result.Status = model.Rebooting
	case bot.IsDead || bot.Quarantined:
		result.Status = model.Offline
	default:
		// Defaults to idle.
	}
	return result, nil
}

func fetch(c context.Context, dim model.Dimensions) (model.MachinePool, error) {
	// Get a Swarming Client first.
	c, _ = context.WithTimeout(c, 10*time.Minute)
	t, err := auth.GetRPCTransport(c, auth.AsSelf)
	if err != nil {
		return nil, err
	}
	sc, err := sv1.New(&http.Client{Transport: t})
	if err != nil {
		return nil, err
	}
	sc.BasePath = fmt.Sprintf("https://%s/_ah/api/swarming/v1/", dim.Host)

	botList, err := sc.Bots.List().Dimensions(dim.Dimensions...).Do()
	if err != nil {
		return nil, err
	}
	result := make([]model.Machine, 0, len(botList.Items))
	for _, bot := range botList.Items {
		// Ignore deleted bots.
		if bot.Deleted {
			continue
		}
		machine, err := botStatus(c, bot)
		if err != nil {
			return nil, err
		}
		result = append(result, *machine)
	}
	return result, nil
}

// machineInfo returns a map of DimensionSHA1 to machine pool.  This data
// is fetched from Swarming in parallel.
func machineInfo(c context.Context, dims []model.Dimensions) (dimInfo map[string]model.MachinePool, err error) {
	dimInfo = make(map[string]model.MachinePool, len(dims))
	pools := make([]model.MachinePool, len(dims))
	err = parallel.FanOutIn(func(ch chan<- func() error) {
		for i, dim := range dims {
			i := i
			dim := dim
			ch <- func() error {
				mp, err := fetch(c, dim)
				if err != nil {
					return err
				}
				pools[i] = mp
				return nil
			}
		}
	})
	if err != nil {
		return
	}
	for i, dim := range dims {
		dimInfo[dim.SHA1] = pools[i]
	}
	return
}

// savePoolInfo saves all of the pool info into datastore.
// We do this one at a time because these entities can get pretty big,
// so batching them can easily go over the datastore API limit of 1.5MB.
func savePoolInfo(c context.Context, builders []model.BuilderPool, poolInfo map[string]model.MachinePool) error {
	for _, builder := range builders {
		machinePool, ok := poolInfo[builder.Dimensions.SHA1]
		if !ok {
			logging.Warningf(
				c, "Did not find bot information for pool with dimensions: %s.  Skipping builder %s",
				builder.Dimensions.Dimensions, builder.BuilderKey.StringID())
			continue
		}
		builder.Machines = machinePool
		if err := datastore.Put(c, &builder); err != nil {
			return err
		}
	}
	return nil
}

func UpdatePools(c context.Context) error {
	host, err := getHost(c)
	if err != nil {
		return err
	}
	sc, err := newSwarmbucketClient(c, host)
	if err != nil {
		return err
	}
	r, err := sc.GetBuilders().Do()
	if err != nil {
		return err
	}
	// Parse the response we get from swarming into a slice of the datastore
	// model entity.  This slice is missing actual machine pool data.
	builderPools := parseBuilders(c, r)
	// Find the unique set of dimentions, so that we only query swarming once
	// for each set of known dimensions.
	dims := uniqueDimensions(builderPools)
	// Fetch the bot pool info from swarming.  One entry per set of dimensions.
	dimInfo, err := machineInfo(c, dims)
	if err != nil {
		return err
	}
	// Merge the data and save it into datastore.
	return savePoolInfo(c, builderPools, dimInfo)
}
