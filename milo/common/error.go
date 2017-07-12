// Copyright 2015 The LUCI Authors.
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

import (
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"
)

// ErrorPage writes an error page into c.Writer with an http code "code"
// and custom message.
func ErrorPage(c *router.Context, code int, message string) {
	c.Writer.WriteHeader(code)
	templates.MustRender(c.Context, c.Writer, "pages/error.html", templates.Args{
		"Code":    code,
		"Message": message,
	})
}
