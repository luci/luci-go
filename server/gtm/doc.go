// Copyright 2023 The LUCI Authors.
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

// Package gtm provides a way to generate Google Tag Manager snippets.
//
// Usage as a server module:
//
//	func main() {
//	  modules := []module.Module{
//	    gtm.NewModuleFromFlags(),
//	  }
//	  server.Main(nil, modules, func(srv *server.Server) error {
//	    srv.Routes.GET("/", ..., func(c *router.Context) {
//	      templates.MustRender(c.Context, c.Writer, "page.html", templates.Args{
//	        "GTMJSSnippet": gtm.JSSnippet(c.Context),
//	        "GTMNoScriptSnippet": gtm.NoScriptSnippet(c.Context),
//	        ...
//	      })
//	    return nil
//	  })
//	  ...
//	}
//
// templates/page.html:
//
//	<html>
//	  <head>
//	    {{.GTMJSSnippet}}
//	    ...
//	  </head>
//	  <body>
//	    {{.GTMNoScriptSnippet}}
//	    ...
//	  </body>
//	</html>
package gtm
