// Copyright 2016 The LUCI Authors.
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

// Package analytics provides a standard way to store the Google Analytics
// tracking ID.
//
// Usage Example
//
// This analytics package provides a convenient way to configure an analytics
// ID for an app.  To use, just call analytics.ID(c) and inject that ID
// into a template.
//
//   import (
//		 ...
//     "github.com/luci/luci-go/server/analytics"
//     ...
//   )
//
//   func myHandler(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
//     ...
//     return templates.Args{
//		   "Analytics": analytics.Snippet(c),
//     }
//   }
//
// And in the base html:
//
// {{ .Analytics }}
package analytics
