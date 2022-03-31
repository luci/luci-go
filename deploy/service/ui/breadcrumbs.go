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

type breadcrumb struct {
	Icon  string
	Title string
	Href  string
	Last  bool
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
	return []breadcrumb{
		{
			Title: "All assets",
			Href:  "/",
		},
		{
			Icon:  ref.Icon,
			Title: ref.Name,
			Href:  ref.Href,
			Last:  true,
		},
	}
}
