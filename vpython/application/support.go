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

package application

import (
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/errors"
)

var appKey = "github.com/luci/luci-go/vpython/application.A"

func withApplication(c context.Context, a *application) context.Context {
	return context.WithValue(c, &appKey, a)
}

func getApplication(c context.Context, args []string) *application {
	a := c.Value(&appKey).(*application)
	a.opts.Args = args
	return a
}

func run(c context.Context, fn func(context.Context) error) int {
	err := fn(c)

	switch t := errors.Unwrap(err).(type) {
	case nil:
		return 0

	case ReturnCodeError:
		return int(t)

	default:
		errors.Log(c, err)
		return 1
	}
}
