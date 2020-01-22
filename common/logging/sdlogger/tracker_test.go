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

package sdlogger

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSeverityTracker(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		st := SeverityTracker{Out: nullOut{}}
		So(st.MaxSeverity(), ShouldEqual, UnknownSeverity)

		st.Write(&LogEntry{Severity: DebugSeverity})
		So(st.MaxSeverity(), ShouldEqual, DebugSeverity)

		st.Write(&LogEntry{Severity: InfoSeverity})
		So(st.MaxSeverity(), ShouldEqual, InfoSeverity)

		st.Write(&LogEntry{Severity: WarningSeverity})
		So(st.MaxSeverity(), ShouldEqual, WarningSeverity)

		st.Write(&LogEntry{Severity: ErrorSeverity})
		So(st.MaxSeverity(), ShouldEqual, ErrorSeverity)

		st.Write(&LogEntry{Severity: WarningSeverity})
		So(st.MaxSeverity(), ShouldEqual, ErrorSeverity) // still error
	})
}

type nullOut struct{}

func (nullOut) Write(*LogEntry) {}
