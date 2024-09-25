// Copyright 2024 The LUCI Authors.
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

package test_helper

import (
	"fmt"
	"strings"
	"testing"
)

type ExpectFailure struct {
	*testing.T

	logCalls []string
	fail     bool
}

var _ testing.TB = (*ExpectFailure)(nil)

func NewExpectFailure(t *testing.T) *ExpectFailure {
	return &ExpectFailure{T: t}
}

func (e *ExpectFailure) Log(args ...any) {
	// fmt.Sprint has special logic to not join strings with " " - TB.Log does not
	// have this logic.
	formatted := make([]string, len(args))
	for i, arg := range args {
		formatted[i] = fmt.Sprint(arg)
	}
	e.logCalls = append(e.logCalls, strings.Join(formatted, " "))
}

func (e *ExpectFailure) Logf(format string, args ...any) {
	e.logCalls = append(e.logCalls, fmt.Sprintf(format, args...))
}

func (e *ExpectFailure) Fail() {
	e.fail = true
}

func (e *ExpectFailure) FailNow() {
	e.fail = true
}

func (e *ExpectFailure) Check(msgs ...string) {
	e.Helper()

	if !e.fail {
		e.T.Log("ExpectFailure: Test case did not call Fail/FailNow.")
		e.T.Fail()
	}

	var missingMsgs []string
	for _, msg := range msgs {
		var ok bool
		for _, logged := range e.logCalls {
			if strings.Contains(logged, msg) {
				ok = true
				break
			}
		}
		if !ok {
			missingMsgs = append(missingMsgs, msg)
		}
	}
	if len(missingMsgs) > 0 {
		e.T.Log("ExpectFailure: Missing Check messages:")
		for _, msg := range missingMsgs {
			e.T.Log(" *", msg)
		}

		e.T.Log("Actual logs:")
		for _, msg := range e.logCalls {
			e.T.Log(msg)
		}
		e.T.Fail()
	}
	if e.T.Failed() {
		e.T.FailNow()
	}
}
