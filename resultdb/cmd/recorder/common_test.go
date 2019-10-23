// Copyright 2019 The LUCI Authors.
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

package main

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExpirations(t *testing.T) {
	Convey(`works`, t, func() {
		invMap := map[string]interface{}{}
		populateExpirations(invMap, time.Date(2017, time.November, 6, 21, 9, 43, 12345678, time.UTC))
		So(invMap, ShouldResemble, map[string]interface{}{
			"InvocationExpirationTime":          time.Date(2019, time.November, 6, 21, 9, 43, 12345678, time.UTC),
			"InvocationExpirationWeek":          time.Date(2019, time.November, 4, 0, 0, 0, 0, time.UTC),
			"ExpectedTestResultsExpirationTime": time.Date(2018, time.January, 5, 21, 9, 43, 12345678, time.UTC),
			"ExpectedTestResultsExpirationWeek": time.Date(2018, time.January, 1, 0, 0, 0, 0, time.UTC),
		})
	})
}
