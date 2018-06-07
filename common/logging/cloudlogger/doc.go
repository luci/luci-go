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
//
// cloudlogger is a /common/logger compatable logger plugin that works with
// Stackdriver logging (aka Google Cloud Logging).
// A supported feature is to be able to combine logs together behind a single
// "HTTP Request".
// StartTrace(context.Context) and EndTrace(context.Context) are used to indicate
// when a request starts and ends.  They're named "Trace" because they set the
// TraceID field in a LogEntry.
//
// To Use:
//
// func doRequest(c context.Context, r *http.Request) {
//   c = cloudlogger.StartTrace(c, r)
//   response := &http.Response{}
//   defer cloudlogger.EndTrace(c, response)
//
//   ... do stuff ...
//   // Use logging as normal
//   logging.Infof(c, "this is an info log")
//   ...
//
//   return
// }
//
// If the runner doesn't use http requests and responses:
//
// func do(c context.Context) (err error) {
//   c = cloudlogger.StartTrace(c, nil)
//   bytesProcessed := 0
//   defer cloudlogger.EndTraceWithError(c, bytesProcessed, err)  // An optional content-length can be specified here.
//
//   ... do stuff ...
//   return
// }

package cloudlogger
