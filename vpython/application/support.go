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
	"context"
	"fmt"
	"os"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/vpython"
)

// returnCodeError is an error wrapping a return code value.
type returnCodeError int

func (err returnCodeError) Error() string {
	return fmt.Sprintf("python interpreter returned non-zero error: %d", err)
}

var appKey = "go.chromium.org/luci/vpython/application.A"

func withApplication(c context.Context, a *application) context.Context {
	return context.WithValue(c, &appKey, a)
}

func getApplication(c context.Context, args []string) *application {
	a := c.Value(&appKey).(*application)
	a.args = args
	return a
}

func run(c context.Context, fn func(context.Context) error) int {
	err := fn(c)

	switch t := errors.Unwrap(err).(type) {
	case nil:
		return 0

	case returnCodeError:
		return int(t)

	default:
		// If the error is tagged as a UserError, just print it without the scary
		// stack trace.
		if vpython.IsUserError.In(err) {
			executable, eErr := os.Executable()
			if eErr != nil {
				executable = "<unknown executable>"
			}
			fmt.Fprintf(os.Stderr, "%s: %s\n", executable, err)
		} else {
			errors.Log(c, err)
		}
		return 1
	}
}
