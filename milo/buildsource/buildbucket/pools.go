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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/milo/buildsource/swarming"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/server/auth"
)

// fullBuilderPool is a struct that combines a model.BuilderPool with its resolved descriptor.
// These are normally separate entities since multiple builders map to a single descriptor,
// but it's more convenient to pass this around internally.
type fullBuilderPool struct {
	model.BuilderPool
	descriptor model.PoolDescriptor
}

// parseBuilders parses out all of the builder pools from the Swarmbucket get_builders response.
// The BuilderPool is tagged with the current time.  It is missing BotPool information,
// which is to be filled later.
func parseBuilders(c context.Context, now time.Time, r *swarmbucket.SwarmingSwarmbucketApiGetBuildersResponseMessage) []fullBuilderPool {
	result := []fullBuilderPool{}
	for _, bucket := range r.Buckets {
		for _, builder := range bucket.Builders {
			id := fmt.Sprintf("buildbucket/%s/%s", bucket.Name, builder.Name)
			descriptor := model.NewPoolDescriptor(bucket.SwarmingHostname, builder.SwarmingDimensions)
			pool := fullBuilderPool{
				BuilderPool: model.BuilderPool{
					BuilderID: datastore.MakeKey(c, "BuilderSummary", id),
					PoolKey:   datastore.MakeKey(c, model.BotPoolKind, descriptor.PoolKey()),
				},
				descriptor: descriptor,
			}
			result = append(result, pool)
		}
	}
	return result
}

// uniqueDescriptors returns all unique set of PoolKeys in the BuilderPool.
// All of the returned keys are of "PoolKeys".
func uniqueDescriptors(p []fullBuilderPool) []model.PoolDescriptor {
	set := map[string]model.PoolDescriptor{}
	for _, pool := range p {
		set[pool.PoolKey.String()] = pool.descriptor
	}
	result := make([]model.PoolDescriptor, 0, len(set))
	for _, key := range set {
		result = append(result, key)
	}
	return result
}

// botStatus parses a Swarming BotInfo response into the structure we will
// save into the datastore.  Since BotInfo doesn't have an explicit status
// field that matches Milo's abstraction of a Bot, the status is inferred:
// * A bot with TaskID is Busy
// * A bot that died in the last 5 minutes is Rebooting
// * A bot that is dead or quarantined is Offline
// * Otherwise, it is implicitly connected and Idle.
func botStatus(c context.Context, bot *sv1.SwarmingRpcsBotInfo) (*model.Bot, error) {
	lastSeen, err := time.Parse(swarming.SwarmingTimeLayout, bot.LastSeenTs)
	if err != nil {
		return nil, err
	}
	result := &model.Bot{
		Name:     bot.BotId,
		LastSeen: lastSeen,
	}

	switch {
	case bot.TaskId != "":
		result.Status = model.Busy
	case bot.IsDead && clock.Now(c).Sub(lastSeen) < 5*time.Minute:
		// Swarming marks a bot as dead immediately while rebooting.
		// We want to give it a 5 minute grace period before we pronounce it dead.
		result.Status = model.Busy
	case bot.IsDead || bot.Quarantined:
		result.Status = model.Offline
	default:
		// Defaults to idle.
	}
	return result, nil
}

// fetchPoolDetails retrieves the Bot pool details for a given set of
// dimensions for its respective Swarming host.
// It makes one call to Swarming's Bots.List endpoint, and returns a list of Machines.
func fetchPoolDetails(c context.Context, desc model.PoolDescriptor) (*model.BotPool, error) {
	// Get a Swarming Client first, this is a cron job so we can run as long as we need to.
	c, _ = context.WithTimeout(c, 10*time.Minute)
	t, err := auth.GetRPCTransport(c, auth.AsSelf)
	if err != nil {
		return nil, err
	}
	sc, err := sv1.New(&http.Client{Transport: t})
	if err != nil {
		return nil, err
	}
	sc.BasePath = fmt.Sprintf("https://%s/_ah/api/swarming/v1/", desc.Host())

	botList, err := sc.Bots.List().Dimensions(desc.Dimensions()...).Do()
	if err != nil {
		return nil, err
	}
	bots := make([]model.Bot, 0, len(botList.Items))
	for _, bot := range botList.Items {
		// Ignore deleted bots.
		if bot.Deleted {
			continue
		}
		Bot, err := botStatus(c, bot)
		if err != nil {
			return nil, err
		}
		bots = append(bots, *Bot)
	}
	return &model.BotPool{
		PoolKey:    desc.PoolKey(),
		Descriptor: desc,
		Bots:       bots,
		LastUpdate: clock.Now(c),
	}, nil
}

// fetchBotPools resolves the descriptors into actual BotPool information
// The input is a list of descriptors to fetch from swarming
// The output is a slice of resolved []BotPool info
// Basically this just runs fetchPoolDetails() a bunch of times.
func fetchBotPools(c context.Context, descriptors []model.PoolDescriptor) (pools []*model.BotPool, err error) {
	pools = make([]*model.BotPool, len(descriptors))
	err = parallel.FanOutIn(func(ch chan<- func() error) {
		for i, desc := range descriptors {
			i := i
			desc := desc
			ch <- func() error {
				mp, err := fetchPoolDetails(c, desc)
				if err != nil {
					return err
				}
				pools[i] = mp
				return nil
			}
		}
	})
	return
}

// savePools saves all of the builder and bot pool info into datastore.
// We do bot pools this one at a time because these entities can get pretty big,
// and batching them can easily go over the datastore API limit of 1.5MB.
func savePools(c context.Context, fullBuilders []fullBuilderPool, bots []*model.BotPool) error {
	// The builders slice should be small, so this can be batched.
	builders := make([]model.BuilderPool, len(fullBuilders))
	for i, b := range fullBuilders {
		builders[i] = b.BuilderPool
	}
	if err := datastore.Put(c, builders); err != nil {
		return errors.Annotate(err, "saving builders").Err()
	}
	for _, pool := range bots {
		if err := datastore.Put(c, pool); err != nil {
			return errors.Annotate(err, "while saving pool: %v").Err()
		}
	}
	return nil
}

// UpdatePools is a cron job endpoint that:
// 1. Fetches all the builders from our associated Swarmbucket instance.
// 2. Consolidates all known descriptors (host+dimensions).
// 3. Fetches bot data from swarming for all known descriptors.
// 4. Saves the data back into the datastore.
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
	// model entity.  This slice is missing actual Bot pool data.
	now := clock.Now(c)
	builderPools := parseBuilders(c, now, r)
	// Find the unique set of descriptors, so that we only query swarming once
	// for each set of known dimensions.
	descriptors := uniqueDescriptors(builderPools)
	// Fetch the bot pool info from swarming.  One entry per descriptor.
	botPools, err := fetchBotPools(c, descriptors)
	if err != nil {
		return err
	}
	// Save all data back into the datastore.
	return savePools(c, builderPools, botPools)
}
