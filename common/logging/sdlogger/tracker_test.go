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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSeverityTracker(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		st := SeverityTracker{Out: nullOut{}}
		assert.Loosely(t, st.MaxSeverity(), should.Equal(UnknownSeverity))

		st.Write(&LogEntry{Severity: DebugSeverity})
		assert.Loosely(t, st.MaxSeverity(), should.Equal(DebugSeverity))

		st.Write(&LogEntry{Severity: InfoSeverity})
		assert.Loosely(t, st.MaxSeverity(), should.Equal(InfoSeverity))

		st.Write(&LogEntry{Severity: WarningSeverity})
		assert.Loosely(t, st.MaxSeverity(), should.Equal(WarningSeverity))

		st.Write(&LogEntry{Severity: ErrorSeverity})
		assert.Loosely(t, st.MaxSeverity(), should.Equal(ErrorSeverity))

		st.Write(&LogEntry{Severity: WarningSeverity})
		assert.Loosely(t, st.MaxSeverity(), should.Equal(ErrorSeverity)) // still error
	})
}

type nullOut struct{}

func (nullOut) Write(*LogEntry) {}
