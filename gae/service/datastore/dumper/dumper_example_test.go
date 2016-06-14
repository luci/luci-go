// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package dumper

import (
	"fmt"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"golang.org/x/net/context"
)

type ExampleModel struct {
	Kind   string         `gae:"$kind,Example"`
	ID     int64          `gae:"$id"`
	Parent *datastore.Key `gae:"$parent"`

	Vals      []string
	Number    int64
	HexNumber int64
}

func ExampleConfig_Query() {
	c := context.Background()
	c = memory.Use(c)

	ds := datastore.Get(c)
	root := ds.MakeKey("Parent", 1)
	models := []*ExampleModel{
		{ID: 1, Vals: []string{"hi", "there"}, Number: 10, HexNumber: 20},
		{ID: 2, Vals: []string{"other"}, Number: 11, HexNumber: 21},
		{ID: 1, Parent: root, Vals: []string{"child", "ent"}},
		{Kind: "Other", ID: 1, Vals: []string{"other"}, Number: 11, HexNumber: 21},
	}
	if err := ds.Put(models); err != nil {
		panic(err)
	}
	// indexes must be up-to-date here.
	ds.Testable().CatchupIndexes()

	_, err := Config{
		PropFilters: PropFilterMap{
			{"Example", "HexNumber"}: func(p datastore.Property) string {
				return fmt.Sprintf("0x%04x", p.Value())
			},
		},
		KindFilters: KindFilterMap{
			"Other": func(key *datastore.Key, pm datastore.PropertyMap) string {
				return "I AM A BANANA"
			},
		},
	}.Query(c, nil)
	if err != nil {
		panic(err)
	}

	// Output:
	// dev~app::/Example,1:
	//   "HexNumber": [0x0014]
	//   "Number": [PTInt(10)]
	//   "Vals": [
	//     PTString("hi"),
	//     PTString("there")
	//   ]
	//
	// dev~app::/Example,2:
	//   "HexNumber": [0x0015]
	//   "Number": [PTInt(11)]
	//   "Vals": [PTString("other")]
	//
	// dev~app::/Other,1:
	//   I AM A BANANA
	//
	// dev~app::/Parent,1/Example,1:
	//   "HexNumber": [0x0000]
	//   "Number": [PTInt(0)]
	//   "Vals": [
	//     PTString("child"),
	//     PTString("ent")
	//   ]
}
