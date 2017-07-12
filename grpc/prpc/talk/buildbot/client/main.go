// Copyright 2015 The LUCI Authors.
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
	"fmt"
	"os"

	"github.com/luci/luci-go/grpc/prpc"
	"github.com/luci/luci-go/grpc/prpc/talk/buildbot/proto"
	"golang.org/x/net/context"
)

func main() {
	ctx := context.Background()
	greeter := buildbot.NewBuildbotPRPCClient(&prpc.Client{Host: "prpc-talk.appspot.com"})

	req := &buildbot.SearchRequest{}
	if len(os.Args) > 1 {
		req.Builder = os.Args[1]
	}

	res, err := greeter.Search(ctx, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, b := range res.Builds {
		fmt.Printf("%s: %s\n", b.Master, b.Builder)
	}
}
