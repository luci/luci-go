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
	"os"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMain(m *testing.M) {
	testBuffer = &messageBufferForTest{}
	os.Exit(m.Run())
}

// TestUnstableSuiteNamesInner ensures that we produce a good error message when
// users do not provide a stable set of names to ftt sub-tests.
func TestUnstableSuiteNamesInner(t *testing.T) {
	externalState := ""
	getNextName := func() string {
		externalState += "a"
		return externalState
	}

	msgBuf := testBuffer.(*messageBufferForTest)
	msgBuf.setupNewTest(false)

	Run("stable", t, func(t *Test) {
		t.Run("another", func(t *Test) {
			t.Run("unrelated", func(t *Test) {
			})
			t.Run(getNextName(), func(t *Test) {
				panic("we should never see this")
			})
			t.Run("extra", func(t *Test) {
			})
		})
	})

	assert.Loosely(t, msgBuf.buf, should.HaveLength(1))
	assert.That(t, msgBuf.buf[0].methodName, should.Equal("Errorf"))
	assert.That(t, msgBuf.buf[0].message, should.ContainSubstring(`Failed to find "stable/another"/ftt.Run("aa")`))
	assert.That(t, msgBuf.buf[0].message, should.ContainSubstring(`found [unrelated aaa extra]`))
}

// TestUnstableSuiteNamesTopLevel ensures that we produce a good error message when
// users do not provide a stable set of names to ftt's root callback.
func TestUnstableSuiteNamesTopLevel(t *testing.T) {
	externalState := ""
	getNextName := func() string {
		externalState += "a"
		return externalState
	}

	msgBuf := testBuffer.(*messageBufferForTest)
	msgBuf.setupNewTest(false)

	Run("stable", t, func(t *Test) {
		t.Run(getNextName(), func(t *Test) {
			panic("we should never see this")
		})
	})

	assert.Loosely(t, msgBuf.buf, should.HaveLength(1))
	assert.That(t, msgBuf.buf[0].methodName, should.Equal("Errorf"))
	assert.That(t, msgBuf.buf[0].message, should.ContainSubstring(`Failed to find "stable"/ftt.Run("a")`))
	assert.That(t, msgBuf.buf[0].message, should.ContainSubstring(`found [aa]`))
}

// TestUnstableSuiteNamesTopLevel ensures that we produce a good error message when
// users re-use the same suite name multiple times at the same level of the
// tree.
func TestDuplicateSuiteNames(t *testing.T) {
	msgBuf := testBuffer.(*messageBufferForTest)
	msgBuf.setupNewTest(false)

	Run("stable", t, func(t *Test) {
		t.Run("someTest", func(t *Test) {
		})
		t.Run("someTest", func(t *Test) {
		})
	})

	assert.Loosely(t, msgBuf.buf, should.HaveLength(2))
	assert.That(t, msgBuf.buf[0].methodName, should.Equal("Fatalf"))
	assert.That(t, msgBuf.buf[0].message, should.ContainSubstring(`Found duplicate test suite`))
	assert.That(t, msgBuf.buf[0].message, should.ContainSubstring(`someTest`))
	assert.That(t, msgBuf.buf[1].methodName, should.Equal("FailNow"))
}
