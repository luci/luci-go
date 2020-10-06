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

// Command rts-chromium is Chromium-specific part of the generic RTS framework.
//
// Install it:
//
//  go install go.chromium.org/luci/rts/cmd/rts-chromium
//
// Primarily rts-chromium can generate history files:
//
//  rts-chromium presubmit-history \
//    -from 2020-10-04 -to 2020-10-05 \
//    -out cq.hist
//
// It will ask to login on the first run.
//
// Filtering
//
// Flags -builder and -test can be used to narrow the search down to specific
// builder and/or test. The flag values are regexps. The following retrieves
// history of browser_tests on linux-rel:
//
//  rts-chromium presubmit-history \
//    -from 2020-10-04 -to 2020-10-05 \
//    -out linux-rel-browser-tests.hist \
//    -builder linux-rel \
//    -test ninja://chrome/test:browser_tests/.+
//
// Test duration fraction
//
// By default the tool fetches only 0.1% of test durations, because
// Chromium CQ produces 1B+ of test results per day. Fetching them all would be
// excessive.
//
// However, if the date range is short and/or filters are applied, the
// default fraction of 0.1% might be inappropriate. It can be changed using
// -duration-data-frac flag. The following changes the sample size to 10%:
//
//  rts-chromium presubmit-history \
//    -from 2020-10-04 -to 2020-10-05 \
//    -out cq.hist
//    -duration-data-frac 0.1
package main
