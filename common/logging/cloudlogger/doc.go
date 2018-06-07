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
//
// To do so:
//
// for {
//   doStuff(c)
// }
//
// func doStuff(c context.Context) (err error) {
//   c, req = cloudlogger.StartTrace(c)
//   req.RemoteIP = ...
//   req.LocalIP = ...
//   req.URL = &url.URL{...}
//   req.UserAgent = ...
//   defer cloudlogger.EndTrace(c)
//
//   ... do stuff here ...
//   req.ResponseSize = ...
//   req.Status = http.StatusOK
//   if err != nil {
//     req.Status = http.StatusInternalServerError
//   }
// }
//
// Everything between StartTrace and EndTrace will get lumped into a single combined log.
// StartTrace returns an optional *Request struct, where the caller can specify optional
// details such as:
// * RemoteIP
// * LocalIP
// * ResponseSize
// * URL
// * User Agent
// * HTTP Status

package cloudlogger
