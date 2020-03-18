// Copyright 2020 The LUCI Authors.
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

package main

import (
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"sync"

	"cloud.google.com/go/storage"
	"golang.org/x/sync/semaphore"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithScopes("https://www.googleapis.com/auth/devstorage.read_only"))
	checkErr(err)
	defer client.Close()

	bkt := client.Bucket("isolateserver")
	it := bkt.Objects(ctx, &storage.Query{
		Prefix: "default-gzip",
	})

	var wg sync.WaitGroup
	s := semaphore.NewWeighted(10)

	for cnt := 0; cnt < 1000; cnt++ {
		a, err := it.Next()
		if err == iterator.Done {
			break
		}
		checkErr(err)
		obj := bkt.Object(a.Name)
		wg.Add(1)

		go func() {
			defer wg.Done()
			s.Acquire(ctx, 1)
			defer s.Release(1)

			r, err := obj.NewReader(ctx)
			checkErr(err)
			defer r.Close()

			gr, err := zlib.NewReader(r)
			checkErr(err)
			defer gr.Close()

			n, err := io.Copy(ioutil.Discard, gr)
			checkErr(err)

			fmt.Println(a.Name, a.Size, n)
		}()
	}

	wg.Wait()
}
