// Copyright 2019 The LUCI Authors.
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

package gkelogger

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSeverityTracker(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		st := SeverityTracker{Out: nullOut{}}
		So(st.MaxSeverity(), ShouldEqual, "")

		st.Write(&LogEntry{Severity: "debug"})
		So(st.MaxSeverity(), ShouldEqual, "debug")

		st.Write(&LogEntry{Severity: "info"})
		So(st.MaxSeverity(), ShouldEqual, "info")

		st.Write(&LogEntry{Severity: "warning"})
		So(st.MaxSeverity(), ShouldEqual, "warning")

		st.Write(&LogEntry{Severity: "error"})
		So(st.MaxSeverity(), ShouldEqual, "error")

		st.Write(&LogEntry{Severity: "warning"})
		So(st.MaxSeverity(), ShouldEqual, "error") // still error
	})
}

type nullOut struct{}

func (nullOut) Write(*LogEntry) {}
