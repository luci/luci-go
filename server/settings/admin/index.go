// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package admin

import (
	"net/http"
	"sort"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
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

func indexPage(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
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
