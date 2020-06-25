package main

import (
	"context"
	"log"
	"os"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/chunker"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/filemetadata"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/tree"
	remoteexecution "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/golang/protobuf/proto"
)

func panicIfErr(err error) {
	if err != nil {
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

	chunkers, ar, err := tree.ComputeOutputsToUpload(os.Args[1], []string{"."}, chunker.DefaultChunkSize, filemetadata.NewNoopCache())
	panicIfErr(err)

	log.Print("before compute", ar)

	td, err := digest.NewFromProto(ar.OutputDirectories[0].TreeDigest)
	panicIfErr(err)

	treeChunker, ok := chunkers[td]
	if !ok {
		log.Fatalf("no chunker for %v", td)
	}

	b, err := treeChunker.FullData()
	panicIfErr(err)

	ret := &remoteexecution.Tree{}
	panicIfErr(proto.Unmarshal(b, ret))

	log.Print("tree ", ret)
	log.Print(digest.NewFromMessage(ret.Root))

	var blobs []*chunker.Chunker
	for _, chunker := range chunkers {
		blobs = append(blobs, chunker)
	}
	tch, err := chunker.NewFromProto(ret.Root, chunker.DefaultChunkSize)
	panicIfErr(err)
	blobs = append(blobs, tch)
	log.Print(len(blobs))

	panicIfErr(c.UploadIfMissing(ctx, blobs...))
	for _, blob := range blobs {
		log.Print(blob.Digest())
	}
	log.Print("finish")

	log.Print(digest.NewFromMessage(ret))
}
