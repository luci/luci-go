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

package ui

import (
	"strings"
	"time"

	"github.com/dustin/go-humanize"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/common"
)

type tagItem struct {
	Title string
	Href  string
	User  string
	Age   string
}

func tagsListing(tags []*repopb.Tag, pkg string, now time.Time) []tagItem {
	l := make([]tagItem, len(tags))
	for i, t := range tags {
		tag := common.JoinInstanceTag(t)
		l[i] = tagItem{
			Title: tag,
			Href:  instancePageURL(pkg, tag),
			User:  strings.TrimPrefix(t.AttachedBy, "user:"),
			Age:   humanize.RelTime(t.AttachedTs.AsTime(), now, "", ""),
		}
	}
	return l
}
