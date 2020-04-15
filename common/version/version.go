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

//go:generate ./update_version.sh

// Package version contains utilities for version.
package version

import "time"

// Time is used for version string.
// By using this variable, we can know how the library, binary are near to up to date.
// This is supposed to be updated periodically.
var Time time.Time

func init() {
	t, err := time.Parse(time.UnixDate, "Wed Apr 15 03:02:08 UTC 2020")
	if err != nil {
		panic(err)
	}
	Time = t
}
