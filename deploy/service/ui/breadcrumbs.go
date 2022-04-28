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

package ui

import (
	"fmt"
)

type breadcrumb struct {
	Icon  string
	Title string
	Href  string
	Last  bool
}

func joinBreadcrumbs(path []breadcrumb, last breadcrumb) []breadcrumb {
	path[len(path)-1].Last = false
	last.Last = true
	return append(path, last)
}

func rootBreadcrumbs() []breadcrumb {
	return []breadcrumb{
		{
			Title: "All assets",
			Href:  "/",
			Last:  true,
		},
	}
}

func assetBreadcrumbs(ref assetRef) []breadcrumb {
	return joinBreadcrumbs(rootBreadcrumbs(), breadcrumb{
		Icon:  ref.Icon,
		Title: ref.Name,
		Href:  ref.Href,
	})
}

func historyListingBreadcrumbs(ref assetRef) []breadcrumb {
	return joinBreadcrumbs(assetBreadcrumbs(ref), breadcrumb{
		Title: "Actuations",
		Href:  fmt.Sprintf("%s/history", ref.Href),
	})
}

func historyEntryBreadcrumbs(ref assetRef, historyID int64) []breadcrumb {
	return joinBreadcrumbs(assetBreadcrumbs(ref), breadcrumb{
		Title: fmt.Sprintf("Actuation #%d", historyID),
		Href:  fmt.Sprintf("%s/history/%d", ref.Href, historyID),
	})
}
