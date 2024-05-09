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

package ftt

import (
	"errors"
	"fmt"
	"sync"
)

type messageBufferEntry struct {
	// methodName is the like Error, Errorf, Log, etc.
	methodName string
	// message is the rendered string message.
	message string
}

var errFailSentinel = errors.New("ftt 'FailNow' emulation; should only be seen in ftt's own tests")

// during tests for ftt itself, this buffers all messages/test failures.
type messageBufferForTest struct {
	mu sync.Mutex

	absorbAllPanics bool
	buf             []messageBufferEntry
	absorbedPanics  []any
}

func (m *messageBufferForTest) Errorf(format string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buf = append(m.buf, messageBufferEntry{"Errorf", fmt.Sprintf(format, args...)})
}

func (m *messageBufferForTest) Fatalf(format string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buf = append(m.buf, messageBufferEntry{"Fatalf", fmt.Sprintf(format, args...)})
	panic(errFailSentinel)
}

func (m *messageBufferForTest) handlePanic() {
	m.mu.Lock()
	defer m.mu.Unlock()

	p := recover()
	if p == nil {
		return
	}
	if p == errFailSentinel {
		m.buf = append(m.buf, messageBufferEntry{
			"FailNow", "Recovered internal errFailSentinel",
		})
		return
	}

	m.buf = append(m.buf, messageBufferEntry{
		"PANIC", "Unknown error recovered in handleFatalPanic; there is a bug.",
	})

	if m.absorbAllPanics {
		m.absorbedPanics = append(m.absorbedPanics, p)
	} else {
		panic(p)
	}
}

func (m *messageBufferForTest) setupNewTest(absorbAllPanics bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.absorbAllPanics = absorbAllPanics
	m.buf = nil
	m.absorbedPanics = nil
}
