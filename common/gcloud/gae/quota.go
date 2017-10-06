// Copyright 2017 The LUCI Authors.
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

// Package gae defines information about the AppEngine environment.
package gae

const (
	// MaxRequestSize is the maximum size of an AppEngine request.
	//
	// https://cloud.google.com/appengine/quotas#Requests
	MaxRequestSize = 32 * 1024 * 1024

	// MaxResponseSize is the maximum size of an AppEngine Standard Environment
	// response.
	//
	// https://cloud.google.com/appengine/docs/standard/python/how-requests-are-handled#response_size_limits
	MaxResponseSize = 32 * 1024 * 1024
)
