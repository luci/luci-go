// Copyright 2018 The LUCI Authors.
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

package gae

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/impl/prod"
	"go.chromium.org/luci/gae/service/datastore"
)

func Example() {
	// This is suitable for use in tests.
	c := memory.Use(context.Background())
	w := httptest.NewRecorder()
	_, err := http.NewRequest("GET", "/doesntmatter", nil)
	if err != nil {
		panic(err)
	}

	innerHandler(c, w)
	fmt.Print(string(w.Body.Bytes()))
	// Output: I wrote: dev~app::/CoolStruct,"struct-id"
}

// This is what you would use in production.
func handler(w http.ResponseWriter, r *http.Request) {
	c := context.Background()
	c = prod.Use(c, r)
	// add production filters, etc. here
	innerHandler(c, w)
}

type CoolStruct struct {
	ID string `gae:"$id"`

	Value string
}

func innerHandler(c context.Context, w http.ResponseWriter) {
	obj := &CoolStruct{ID: "struct-id", Value: "hello"}
	if err := datastore.Put(c, obj); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	fmt.Fprintf(w, "I wrote: %s", datastore.KeyForObj(c, obj))
}
