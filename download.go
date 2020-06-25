package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	_ "google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/status"
)

func panicIfErr(err error) {
	if err != nil {
		log.Printf("%+#v", err)
		panic(err)
	}
}

func main() {
	ctx := context.Background()

	c, err := client.NewClient(ctx, "projects/chromium-swarm-dev/instances/default_instance",
		client.DialParams{
			Service:               "remotebuildexecution.googleapis.com:443",
			UseApplicationDefault: true,
		})
	panicIfErr(err)
	defer c.Close()
	log.Print("before download")

	// digest of root directory
	d, err := digest.NewFromString(os.Args[1])
	panicIfErr(err)

	res, err := c.GetDirectoryTree(ctx, d.ToProto())
	if s, ok := status.FromError(err); ok {
		fmt.Println(s.Details())
	}
	panicIfErr(err)

	root := &repb.Directory{}
	panicIfErr(c.ReadProto(ctx, d, root))

	t := &repb.Tree{
		Root:     root,
		Children: res[1:],
	}
	log.Print("tree ", t)

	td, err := digest.NewFromMessage(t)
	panicIfErr(err)

	ar := &repb.ActionResult{
		OutputDirectories: []*repb.OutputDirectory{
			{
				Path:       "action-out",
				TreeDigest: td.ToProto(),
			},
		},
	}
	log.Print(ar)
	panicIfErr(c.DownloadActionOutputs(ctx, ar, "."))
}
