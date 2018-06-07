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
//   c = cloudlogger.StartTrace(c)
//
//   ... Do stuff here ...
//
//   cloudlogger.EndTrace(c, nil).Infof(c, "Request finished")
// }
//
// Everything between StartTrace and EndTrace will get lumped into a single combined log.
// EndTrace takes an optional *Request struct, where the caller can specify optional
// details such as:
// * RemoteIP
// * LocalIP
// * ResponseSize
// * URL
// * User Agent
// * HTTP Status

package cloudlogger
