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

package ui

import (
	"context"
	"net/url"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/milo/common/model"
)

// BuildSummary is a summary of a build, with just enough information for display
// on a builder page.
type BuildSummary struct {
	// Link to the build.
	Link *Link

	// Status of the build.
	Status model.Status

	// Pending is time interval that this build was pending.
	PendingTime Interval

	// Execution is time interval that this build was executing.
	ExecutionTime Interval

	// Revision is the main revision of the build.
	Revision *Commit

	// Arbitrary text to display below links.  One line per entry,
	// newlines are stripped.
	Text []string

	// Blame is for tracking whose change the build belongs to, if any.
	Blame []*Commit
}

// Builder denotes an ordered list of MiloBuilds
type Builder struct {
	// Name of the builder
	Name string

	// Indicates that this Builder should render a blamelist for each build.
	// This is only supported for Buildbot (crbug.com/807846)
	HasBlamelist bool

	// Warning text, if any.
	Warning string

	CurrentBuilds []*BuildSummary
	PendingBuilds []*BuildSummary
	// PendingBuildNum is the number of pending builds, since the slice above
	// may be a snapshot instead of the full set.
	PendingBuildNum int
	FinishedBuilds  []*BuildSummary

	// MachinePool is primarily used by buildbot builders to list the set of
	// machines that can run in a builder.  It has no meaning in buildbucket or dm
	// and is expected to be nil.
	MachinePool *MachinePool

	// Groups is a list of links to builder groups that contain this builder.
	Groups []*Link

	// PrevCursor is a cursor to the previous page.
	PrevCursor string `json:",omitempty"`
	// NextCursor is a cursor to the next page.
	NextCursor string `json:",omitempty"`
}

// Bot wraps a model.Bot to provide a UI specific statuses.
type Bot struct {
	model.Bot
	Status model.BotStatus
}

func (b *Bot) Label() *Link {
	if b == nil {
		return nil
	}
	return NewLink(b.Name, b.URL, "bot "+b.Name)
}

// MachinePool represents the capacity and availability of a builder.
type MachinePool struct {
	Total        int
	Offline      int
	Idle         int
	Busy         int
	Bots         []Bot
	Dimensions   []string
	SwarmingHost string
}

// SwarmingURL returns the swarming bot URL for the machine pool, if available.
func (mp *MachinePool) SwarmingURL() string {
	if mp.SwarmingHost == "" {
		return ""
	}
	u := &url.URL{
		Scheme: "https",
		Host:   mp.SwarmingHost,
		Path:   "botlist",
		RawQuery: url.Values{
			"f": mp.Dimensions,
		}.Encode(),
	}
	return u.String()
}

// NewMachinePool calculates stats from a model.Bot and generates a MachinePool.
// This requires a context because setting the UI Status field requires the current time.
func NewMachinePool(c context.Context, botPool *model.BotPool) *MachinePool {
	fiveMinAgo := clock.Now(c).Add(-time.Minute * 5)
	result := &MachinePool{
		Total:        len(botPool.Bots),
		Bots:         make([]Bot, len(botPool.Bots)),
		Dimensions:   botPool.Descriptor.Dimensions().Format(),
		SwarmingHost: botPool.Descriptor.Host(),
	}
	for i, bot := range botPool.Bots {
		uiBot := Bot{bot, bot.Status} // Wrap the model.Bot
		if bot.Status == model.Offline && bot.LastSeen.After(fiveMinAgo) {
			// If the bot has been offline for less than 5 minutes, mark it as busy.
			uiBot.Status = model.Busy
		}
		switch bot.Status {
		case model.Idle:
			result.Idle++
		case model.Busy:
			result.Busy++
		case model.Offline:
			result.Offline++
		}
		result.Bots[i] = uiBot
	}
	return result
}
