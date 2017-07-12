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

package apigen

// directoryList is a Google Cloud Endpoints frontend directory list structure.
//
// This is the first-level directory structure, which exports a series of API
// items.
type directoryList struct {
	Kind             string           `json:"kind,omitempty"`
	DiscoveryVersion string           `json:"discoveryVersion,omitempty"`
	Items            []*directoryItem `json:"items,omitempty"`
}

// directoryItem is a single API's directoryList entry.
//
// This is a This is the second-level directory structure which exports a single
// API's methods.
//
// The directoryItem exports a REST API (restDescription) at its relative
// DiscoveryLink.
type directoryItem struct {
	Kind             string `json:"kind,omitempty"`
	ID               string `json:"id,omitempty"`
	Name             string `json:"name,omitempty"`
	Version          string `json:"version,omitempty"`
	Title            string `json:"title,omitempty"`
	Description      string `json:"description,omitempty"`
	DiscoveryRestURL string `json:"discoveryRestUrl,omitempty"`
	DiscoveryLink    string `json:"discoveryLink,omitempty"`
	RootURL          string `json:"rootUrl,omitempty"`
	Preferred        bool   `json:"preferred,omitempty"`
}
