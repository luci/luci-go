// Copyright 2016 The LUCI Authors.
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

package admin

import (
	"sort"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/settings"
	"github.com/luci/luci-go/server/templates"
)

type pageIndexEntry struct {
	ID    string
	Title string
}

type pageIndexEntries []pageIndexEntry

func (a pageIndexEntries) Len() int           { return len(a) }
func (a pageIndexEntries) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a pageIndexEntries) Less(i, j int) bool { return a[i].Title < a[j].Title }

func indexPage(ctx *router.Context) {
	c, rw := ctx.Context, ctx.Writer

	entries := pageIndexEntries{}
	for id, p := range settings.GetUIPages() {
		title, err := p.Title(c)
		if err != nil {
			replyError(c, rw, err)
			return
		}
		entries = append(entries, pageIndexEntry{
			ID:    id,
			Title: title,
		})
	}
	sort.Sort(entries)

	// Grab timestamp when last settings change hits all instances.
	consistencyTime := time.Time{}
	if s := settings.GetSettings(c); s != nil {
		if storage, _ := s.GetStorage().(settings.EventualConsistentStorage); storage != nil {
			var err error
			if consistencyTime, err = storage.GetConsistencyTime(c); err != nil {
				replyError(c, rw, err)
				return
			}
		}
	}

	now := clock.Now(c).UTC()
	templates.MustRender(c, rw, "pages/index.html", templates.Args{
		"Entries":               entries,
		"WaitingForConsistency": !consistencyTime.IsZero() && now.Before(consistencyTime),
		"TimeToConsistency":     humanize.RelTime(consistencyTime, now, "", ""),
	})
}
