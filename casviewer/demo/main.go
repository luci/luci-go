package main

import (
	"context"
	"fmt"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/credentials/oauth"
)

func main() {
	ctx := context.Background()

	instance := "projects/chromium-swarm-dev/instances/default_instance"
	perRPCCreds, err := oauth.NewApplicationDefault(ctx)
	if err != nil {
		panic(err)
	}

	client, err := client.NewClient(ctx, instance,
		client.DialParams{
			Service:            "remotebuildexecution.googleapis.com:443",
			TransportCredsOnly: true,
		}, &client.PerRPCCreds{Creds: perRPCCreds})
	if err != nil {
		panic(err)
	}
	rootDigests := []string{
		"/blobs/111aa690b079d1503c73812e66136e76a86bc86077ebdac0f72bc7def5986f2f/84",
		"/blobs/d5cb3ac930a2175975b1604a454d83ddfeee6cffc21de078d0ad297e892a6bd3/78",
	}
	for _, rd := range rootDigests {
		n := instance + rd
		rootBytes, err := client.ReadBytes(ctx, n)
		if err != nil {
			panic(err)
		}
		rootDir := &repb.Directory{}
		if err = proto.Unmarshal(rootBytes, rootDir); err != nil {
			panic(err)
		}
		fmt.Printf("%v\n", rootDir.GetDirectories())
	}
}
