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

package ftt_test

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/testing/typed"
)

// TestFttTraversal shows you the traversal order. Note how many times "1" gets re-executed.
func TestFttTraversal(t *testing.T) {
	t.Parallel()

	var items []int

	expected := []int{
		1,
		1, 2,
		1, 2, 3,
		1, 2, 4,
		1, 5,
		1, 5, 6,
		1, 5, 7,
	}

	ftt.Run("A", t, func(t *ftt.Test) {
		items = append(items, 1)
		t.Run("B", func(*ftt.Test) {
			items = append(items, 2)
			t.Run("C", func(*ftt.Test) {
				items = append(items, 3)
			})
			t.Run("D", func(*ftt.Test) {
				items = append(items, 4)
			})
		})
		t.Run("E", func(*ftt.Test) {
			items = append(items, 5)
			t.Run("F", func(*ftt.Test) {
				items = append(items, 6)
			})
			t.Run("G", func(*ftt.Test) {
				items = append(items, 7)
			})
		})
	})

	if diff := typed.Got(items).Want(expected).Diff(); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}
}

func TestFtt(t *testing.T) {
	t.Parallel()

	order := []string{}

	ftt.Run("root", t, func(t *ftt.Test) {
		order = append(order, "ROOT:"+t.Name())

		t.Run("something", func(t *ftt.Test) {
			order = append(order, "SOMETHING:"+t.Name())
		})

		t.Run("else", func(t *ftt.Test) {
			order = append(order, "ELSE:"+t.Name())
		})
	})

	ftt.Run("root2", t, func(t *ftt.Test) {
		order = append(order, "ROOT2:"+t.Name())

		t.Run("bonus suite", func(t *ftt.Test) {
			order = append(order, "BONUS:"+t.Name())
		})
	})

	expect := []string{
		"ROOT:TestFtt/root",

		"ROOT:TestFtt/root/something",
		"SOMETHING:TestFtt/root/something",

		"ROOT:TestFtt/root/else",
		"ELSE:TestFtt/root/else",

		"ROOT2:TestFtt/root2",

		"ROOT2:TestFtt/root2/bonus_suite",
		"BONUS:TestFtt/root2/bonus_suite",
	}

	assert.That(t, order, should.Match(expect))
}
