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
)

type refItem struct {
	Title string
	Href  string
	User  string
	Age   string
}

func refsListing(refs []*repopb.Ref, pkg string, now time.Time) []refItem {
	l := make([]refItem, len(refs))
	for i, r := range refs {
		l[i] = refItem{
			Title: r.Name,
			Href:  instancePageURL(pkg, r.Name),
			User:  strings.TrimPrefix(r.ModifiedBy, "user:"),
			Age:   humanize.RelTime(r.ModifiedTs.AsTime(), now, "", ""),
		}
	}
	return l
}
