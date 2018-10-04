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
	"context"
	"fmt"
	"net/http"
	"time"

	"go.chromium.org/gae/service/datastore"
	swarmbucketAPI "go.chromium.org/luci/common/api/buildbucket/swarmbucket/v1"
	swarmingAPI "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/milo/buildsource/swarming"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/frontend/ui"
	"go.chromium.org/luci/server/auth"
)

func getPool(c context.Context, bid BuilderID) (*ui.MachinePool, error) {
	// Get PoolKey
	builderPool := model.BuilderPool{
		BuilderID: datastore.MakeKey(c, model.BuilderSummaryKind, bid.String()),
	}
	// These are eventually consistent, so just log an error and pass if not found.
	switch err := datastore.Get(c, &builderPool); {
	case datastore.IsErrNoSuchEntity(err):
		logging.Warningf(c, "builder pool not found")
		return nil, nil
	case err != nil:
		return nil, err
	}
	// Get BotPool
	botPool := model.BotPool{PoolID: builderPool.PoolKey.StringID()}
	switch err := datastore.Get(c, &botPool); {
	case datastore.IsErrNoSuchEntity(err):
		logging.Warningf(c, "bot pool not found")
		return nil, nil
	case err != nil:
		return nil, err
	}
	return ui.NewMachinePool(c, botPool.Bots), nil
}

// processBuilders parses out all of the builder pools from the Swarmbucket get_builders response,
// and saves the BuilderPool information into the datastore.
// It returns a list of PoolDescriptors that needs to be fetched and saved.
func processBuilders(c context.Context, r *swarmbucketAPI.SwarmingSwarmbucketApiGetBuildersResponseMessage) ([]model.PoolDescriptor, error) {
	var builderPools []model.BuilderPool
	var descriptors []model.PoolDescriptor
	seen := stringset.New(0)
	for _, bucket := range r.Buckets {
		for _, builder := range bucket.Builders {
			bid := NewBuilderID(bucket.Name, builder.Name)
			if bid.Project == "" {
				// This may happen if the bucket or builder does not fulfill the LUCI
				// naming convention.
				logging.Warningf(c, "invalid bucket/builder %q/%q, skipping", bucket.Name, builder.Name)
				continue
			}
			id := bid.String()
			descriptor := model.NewPoolDescriptor(bucket.SwarmingHostname, builder.SwarmingDimensions)
			dID := descriptor.PoolID()
			builderPools = append(builderPools, model.BuilderPool{
				BuilderID: datastore.MakeKey(c, model.BuilderSummaryKind, id),
				PoolKey:   datastore.MakeKey(c, model.BotPoolKind, dID),
			})
			if added := seen.Add(dID); added {
				descriptors = append(descriptors, descriptor)
			}
		}
	}
	return descriptors, datastore.Put(c, builderPools)
}

// parseBot parses a Swarming BotInfo response into the structure we will
// save into the datastore.  Since BotInfo doesn't have an explicit status
// field that matches Milo's abstraction of a Bot, the status is inferred:
// * A bot with TaskID is Busy
// * A bot that is dead or quarantined is Offline
// * Otherwise, it is implicitly connected and Idle.
func parseBot(c context.Context, swarmingHost string, botInfo *swarmingAPI.SwarmingRpcsBotInfo) (*model.Bot, error) {
	lastSeen, err := time.Parse(swarming.SwarmingTimeLayout, botInfo.LastSeenTs)
	if err != nil {
		return nil, err
	}
	result := &model.Bot{
		Name:     botInfo.BotId,
		URL:      fmt.Sprintf("https://%s/bot?id=%s", swarmingHost, botInfo.BotId),
		LastSeen: lastSeen,
	}

	switch {
	case botInfo.TaskId != "":
		result.Status = model.Busy
	case botInfo.IsDead || botInfo.Quarantined:
		result.Status = model.Offline
	default:
		// Defaults to idle.
	}
	return result, nil
}

// processBot retrieves the Bot pool details from Swarming for a given set of
// dimensions for its respective Swarming host, and saves the data into datastore.
func processBot(c context.Context, desc model.PoolDescriptor) error {
	t, err := auth.GetRPCTransport(c, auth.AsSelf)
	if err != nil {
		return err
	}
	sc, err := swarmingAPI.New(&http.Client{Transport: t})
	if err != nil {
		return err
	}
	sc.BasePath = fmt.Sprintf("https://%s/_ah/api/swarming/v1/", desc.Host())

	var bots []model.Bot
	bl := sc.Bots.List().Dimensions(desc.Dimensions().Format()...)
	// Keep fetching until the cursor is empty.
	for {
		botList, err := bl.Do()
		if err != nil {
			return err
		}
		for _, botInfo := range botList.Items {
			// Ignore deleted bots.
			if botInfo.Deleted {
				continue
			}
			bot, err := parseBot(c, desc.Host(), botInfo)
			if err != nil {
				return err
			}
			bots = append(bots, *bot)
		}

		if botList.Cursor == "" {
			break
		}
		bl = bl.Cursor(botList.Cursor)
	}
	// This is a large RPC, don't try to batch it.
	return datastore.Put(c, &model.BotPool{
		PoolID:     desc.PoolID(),
		Descriptor: desc,
		Bots:       bots,
		LastUpdate: clock.Now(c),
	})
}

// fetchBotPools resolves the descriptors into actual BotPool information.
// The input is a list of descriptors to fetch from swarming.
// Basically this just runs processBot() a bunch of times.
func processBots(c context.Context, descriptors []model.PoolDescriptor) error {
	return parallel.WorkPool(8, func(ch chan<- func() error) {
		for _, desc := range descriptors {
			desc := desc
			ch <- func() error {
				return processBot(c, desc)
			}
		}
	})
}

// UpdatePools is a cron job endpoint that:
// 1. Fetches all the builders from our associated Swarmbucket instance.
// 2. Consolidates all known descriptors (host+dimensions), saves BuilderPool.
// 3. Fetches and saves BotPool data from swarming for all known descriptors.
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
	// Process builders and save them.  We get back the descriptors that we have
	// to fetch next.
	descriptors, err := processBuilders(c, r)
	if err != nil {
		return errors.Annotate(err, "processing builders").Err()
	}
	// And now also fetch and save the BotPools.
	return processBots(c, descriptors)
}
