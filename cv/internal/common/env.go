// Copyright 2020 The LUCI Authors.
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

package common

// Env describes where CV runs at.
type Env struct {
	// LogicalHostname is CV hostname referred to in configs.
	//
	// On GAE, this is something like "luci-change-verifier-dev.appspot.com"
	// and it is part of the HTTPAddressBase.
	//
	// Under local development, this is usually set to GAE-looking hostname, while
	// keeping HTTPAddressBase a real localhost URL.
	LogicalHostname string

	// HTTPAddressBase can be used to generate URLs to this CV service.
	//
	// Doesn't have a trailing slash.
	//
	// For example,
	//   * "https://luci-change-verifier-dev.appspot.com"
	//   * "http://localhost:8080"
	HTTPAddressBase string

	// IsGAEDev is true if this is a -dev GAE environment.
	//
	// Deprecated. Do not use in new code. It should only be used during migration
	// from CQDaemon which doesn't have equivalent -dev environment.
	IsGAEDev bool
}
