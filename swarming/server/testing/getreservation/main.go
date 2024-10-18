// Copyright 2023 The LUCI Authors.
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

// Command getreservation fetches reservation status from RBE.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"

	"go.chromium.org/luci/swarming/internal/remoteworkers"
)

var (
	wait = flag.Duration("wait", 0, "How long to wait for completion")
)

func main() {
	flag.Parse()
	ctx := gologger.StdConfig.Use(context.Background())
	if err := run(ctx, flag.Args()); err != nil {
		errors.Log(ctx, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.Reason("need one positional argument with full reservation name").Err()
	}
	reservationName := args[0]

	creds, err := auth.NewAuthenticator(ctx, auth.SilentLogin, chromeinfra.SetDefaultAuthOptions(auth.Options{
		Scopes: []string{
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/userinfo.email",
		},
	})).PerRPCCredentials()
	if err != nil {
		return err
	}

	cc, err := grpc.NewClient("remotebuildexecution.googleapis.com:443",
		grpc.WithTransportCredentials(credentials.NewTLS(nil)),
		grpc.WithPerRPCCredentials(creds),
	)
	if err != nil {
		return err
	}
	rbe := remoteworkers.NewReservationsClient(cc)

	reservation, err := rbe.GetReservation(ctx, &remoteworkers.GetReservationRequest{
		Name:        reservationName,
		WaitSeconds: int64(wait.Seconds()),
	})
	if err != nil {
		return err
	}

	text, err := (prototext.MarshalOptions{Multiline: true, Indent: "  "}).Marshal(reservation)
	if err != nil {
		return err
	}

	fmt.Printf("%s\n", text)
	return nil
}
