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
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"io/ioutil"
	"os"

	gaeds "cloud.google.com/go/datastore"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/buildbucket/appengine/model"
	bbpb "go.chromium.org/luci/buildbucket/proto"

	"go.chromium.org/luci/gae/impl/cloud"
	"go.chromium.org/luci/gae/service/datastore"
)

func main() {
	ctx := useDS(context.Background())

	m := &model.BuildSteps{
		Build: datastore.MakeKey(ctx, "Build", 8854510223344790752),
	}
	if err := datastore.Get(ctx, m); err != nil {
		fail("get steps", err)
	}

	r, err := zlib.NewReader(bytes.NewReader(m.Bytes))
	if err != nil {
		fail("create zlib reader", err)
	}
	raw, err := ioutil.ReadAll(r)
	if err != nil {
		fail("read bytes", err)
	}
	if err := r.Close(); err != nil {
		fail("close zlib reader", err)
	}

	p := &bbpb.Build{}
	if err := proto.Unmarshal(raw, p); err != nil {
		fail("unmarshal", err)
	}
	p.Steps = p.Steps[:len(p.Steps)-1]
	b, err := proto.Marshal(p)
	if err != nil {
		fail("marshal", err)
	}
	buf := &bytes.Buffer{}
	w := zlib.NewWriter(buf)
	if _, err := w.Write(b); err != nil {
		fail("zlib", err)
	}
	if err := w.Close(); err != nil {
		fail("close", err)
	}
	m.Bytes = buf.Bytes()
	if err := datastore.Put(ctx, m); err != nil {
		fail("put", err)
	}
}

func fail(message string, err error) {
	fmt.Printf("error when %s: %s", message, err)
	os.Exit(1)
}

func useDS(ctx context.Context) context.Context {
	client, err := gaeds.NewClient(ctx, "cr-buildbucket")
	if err != nil {
		fail("init DS", err)
	}
	cfg := &cloud.ConfigLite{
		ProjectID: "cr-buildbucket",
		DS:        client,
	}

	return cfg.Use(ctx)
}
