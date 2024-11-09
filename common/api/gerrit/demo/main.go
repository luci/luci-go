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

// This is a bare bone script that interact with Gerrit using Gerrit REST
// client for testing purpose. It calls GetChange with ChangeId 1111 by
// default. Modify the api to call and the input to the api in
// `callAndPrintResp` func to test your change.

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

const gerritHost = "chromium-review.googlesource.com"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	go func() {
		<-interrupt
		cancel()
	}()

	runWithCtx(ctx)
}

func runWithCtx(ctx context.Context) {
	client, err := gerrit.NewRESTClient(&http.Client{}, gerritHost, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error while creating gerrit REST client: %s\n", err)
		os.Exit(1)
	}
	errC := make(chan error)
	go func() {
		errC <- callAndPrintResp(ctx, client)
	}()
	select {
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr, ctx.Err())
		os.Exit(1)
	case err := <-errC:
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func callAndPrintResp(ctx context.Context, client gerritpb.GerritClient) error {
	resp, err := client.GetChange(ctx, &gerritpb.GetChangeRequest{
		Number: 2653292,
		Options: []gerritpb.QueryOption{
			gerritpb.QueryOption_DETAILED_LABELS,
			gerritpb.QueryOption_DETAILED_ACCOUNTS,
		},
	})
	if err != nil {
		return errors.Annotate(err, "calling gerrit").Err()
	}

	m := &protojson.MarshalOptions{
		Indent: "  ",
	}
	b, err := m.Marshal(resp)
	if err != nil {
		return errors.Annotate(err, "marshal response proto").Err()
	}
	_, err = os.Stdout.Write(b)
	return err
}
