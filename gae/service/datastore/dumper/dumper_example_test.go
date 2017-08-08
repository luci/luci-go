// Copyright 2016 The LUCI Authors.
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

package dumper

import (
	"fmt"

	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"

	"golang.org/x/net/context"
)

type ExampleModel struct {
	Kind   string  `gae:"$kind,Example"`
	ID     int64   `gae:"$id"`
	Parent *ds.Key `gae:"$parent"`

	Vals      []string
	Number    int64
	HexNumber int64
}

func ExampleConfig_Query() {
	c := context.Background()
	c = memory.Use(c)

	root := ds.MakeKey(c, "Parent", 1)
	models := []*ExampleModel{
		{ID: 1, Vals: []string{"hi", "there"}, Number: 10, HexNumber: 20},
		{ID: 2, Vals: []string{"other"}, Number: 11, HexNumber: 21},
		{ID: 1, Parent: root, Vals: []string{"child", "ent"}},
		{Kind: "Other", ID: 1, Vals: []string{"other"}, Number: 11, HexNumber: 21},
	}
	if err := ds.Put(c, models); err != nil {
		panic(err)
	}
	// indexes must be up-to-date here.
	ds.GetTestable(c).CatchupIndexes()

	_, err := Config{
		PropFilters: PropFilterMap{
			{"Example", "HexNumber"}: func(p ds.Property) string {
				return fmt.Sprintf("0x%04x", p.Value())
			},
		},
		KindFilters: KindFilterMap{
			"Other": func(key *ds.Key, pm ds.PropertyMap) string {
				return "I AM A BANANA"
			},
		},
	}.Query(c, nil)
	if err != nil {
		panic(err)
	}

	// Output:
	// dev~app::/Example,1:
	//   "HexNumber": 0x0014
	//   "Number": PTInt(10)
	//   "Vals": [
	//     PTString("hi"),
	//     PTString("there")
	//   ]
	//
	// dev~app::/Example,2:
	//   "HexNumber": 0x0015
	//   "Number": PTInt(11)
	//   "Vals": [PTString("other")]
	//
	// dev~app::/Other,1:
	//   I AM A BANANA
	//
	// dev~app::/Parent,1/Example,1:
	//   "HexNumber": 0x0000
	//   "Number": PTInt(0)
	//   "Vals": [
	//     PTString("child"),
	//     PTString("ent")
	//   ]
}
