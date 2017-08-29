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

package resp

import (
	"testing"

	"go.chromium.org/luci/milo/common/model"

	. "github.com/smartystreets/goconvey/convey"
)

func createTestBuilderRef() *BuilderRef {
	return &BuilderRef{
		Name:      "test",
		ShortName: "t",
		Build:     make([]*model.BuildSummary, 0),
	}
}

func TestCategory(t *testing.T) {
	Convey("Category structure", t, func() {
		Convey("Root category", func() {
			root := NewCategory("root")
			So(len(root.Children), ShouldEqual, 0)
			So(root.Name, ShouldEqual, "root")
		})

		Convey("With builder", func() {
			root := NewCategory("")
			br := createTestBuilderRef()
			var categories []string
			root.AddBuilder(categories, br)
			So(len(root.Children), ShouldEqual, 1)
			So(root.Children[0].(*BuilderRef).Name, ShouldEqual, br.Name)
		})

		Convey("With nested categories", func() {
			root := NewCategory("")
			br := createTestBuilderRef()
			root.AddBuilder([]string{"cat1", "cat2"}, br)
			So(len(root.Children), ShouldEqual, 1)
			child1 := root.Children[0].(*Category)
			So(len(child1.Children), ShouldEqual, 1)
			So(child1.Name, ShouldEqual, "cat1")
			child2 := child1.Children[0].(*Category)
			So(len(child2.Children), ShouldEqual, 1)
			So(child2.Name, ShouldEqual, "cat2")
			So(child2.Children[0].(*BuilderRef).Name, ShouldEqual, br.Name)
		})

		Convey("Multiple categories", func() {
			root := NewCategory("")
			br1 := createTestBuilderRef()
			br2 := createTestBuilderRef()
			root.AddBuilder([]string{"cat1"}, br1)
			root.AddBuilder([]string{"cat2"}, br2)
			So(len(root.Children), ShouldEqual, 2)
			child1 := root.Children[0].(*Category)
			So(len(child1.Children), ShouldEqual, 1)
			So(child1.Children[0].(*BuilderRef).Name, ShouldEqual, br1.Name)
			child2 := root.Children[1].(*Category)
			So(len(child2.Children), ShouldEqual, 1)
			So(child2.Children[0].(*BuilderRef).Name, ShouldEqual, br2.Name)
		})

		Convey("Reusing existing categories", func() {
			root := NewCategory("")
			br1 := createTestBuilderRef()
			br2 := createTestBuilderRef()
			root.AddBuilder([]string{"cat1"}, br1)
			root.AddBuilder([]string{"cat1"}, br2)
			So(len(root.Children), ShouldEqual, 1)
			child := root.Children[0].(*Category)
			So(len(child.Children), ShouldEqual, 2)
			So(child.Children[0].(*BuilderRef).Name, ShouldEqual, br1.Name)
			So(child.Children[1].(*BuilderRef).Name, ShouldEqual, br2.Name)
		})
	})
}
