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

package convey

import (
	"fmt"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/truth/should"
)

type myType struct {
	strVal string
	intVal int

	ignoreInternalCallback func()
}

func shouldMatchMyType(actual any, expected ...any) string {
	// NOTE: Propery convey assertions should be able to return JSON in
	// their return value. This is incredibly tedious to implement, so most
	// custom comparisons don't do this.

	if len(expected) > 1 {
		return convey.ShouldHaveLength(expected, 1)
	}
	mine, ok := actual.(myType)
	if !ok {
		return fmt.Sprintf("actual must be myType, got %T", actual)
	}
	other, ok := expected[0].(myType)
	if !ok {
		return fmt.Sprintf("other must be myType, got %T", expected[0])
	}
	if mine.strVal != other.strVal {
		return fmt.Sprintf(
			"actual.strVal != other.strVal: %q vs %q",
			mine.strVal, other.strVal)
	}

	if mine.intVal != other.intVal {
		return fmt.Sprintf(
			"actual.intVal != other.intVal: %d vs %d",
			mine.intVal, other.intVal)
	}
	return ""
}

func TestAdapt(t *testing.T) {
	exampleMyTypeFromProdCode := myType{intVal: 100, ignoreInternalCallback: func() {}}

	// have to use assert.Loosely because Adapt returns comparison.Func[any]
	assert.Loosely(t, exampleMyTypeFromProdCode, Adapt(shouldMatchMyType)(myType{intVal: 100}))

	// check for failures
	assert.That(t, Adapt(shouldMatchMyType)(myType{intVal: 100})("smarfle"), should.Resemble(&failure.Summary{
		Comparison: &failure.Comparison{
			Name: "adapt.Convey(go.chromium.org/luci/common/testing/truth/convey.shouldMatchMyType)",
		},
		Findings: []*failure.Finding{
			{Name: "Because", Value: []string{"actual must be myType, got string"}},
		},
	}))
	assert.That(t, Adapt(shouldMatchMyType)(myType{intVal: 100})(myType{strVal: "100"}), should.Resemble(&failure.Summary{
		Comparison: &failure.Comparison{
			Name: "adapt.Convey(go.chromium.org/luci/common/testing/truth/convey.shouldMatchMyType)",
		},
		Findings: []*failure.Finding{
			{Name: "Because", Value: []string{`actual.strVal != other.strVal: "100" vs ""`}},
		},
	}))
}
