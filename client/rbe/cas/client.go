package cas

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"google.golang.org/api/support/bundler"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"

	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	bsgrpc "google.golang.org/genproto/googleapis/bytestream"
)

var zstdEncoders = &sync.Pool{
	New: func() interface{} {
		enc, err := zstd.NewWriter(nil)
		if err != nil {
			panic(err) // Impossible
		}
		return enc
	},
}

//var zstdEncoder, _ = zstd.NewWriter(nil)

type ClientConfig struct {
	DialParams client.DialParams
	// MaxBatchSize is maximum size in bytes of a batch request for batch operations.
	MaxBatchSize int64
	// MaxBatchDigests is maximum amount of digests to batch in batched operations.
	MaxBatchDigests int
	// MaxFindMissingDigestsSize maximum number of digests to check in a single FindMissing RPC.
	MaxFindMissingDigestsSize int
	// SmallFileThreshold is the threshold to decide that the file is small enough
	// to buffer it in memory, as opposed to re-reading it later.
	SmallFileThreshold      int
	BatchUploadConcurrency  int
	StreamUploadConcurrency int
	SmallFileIOConcurrency  int
	Retrier                 *client.Retrier
}

func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		DialParams: client.DialParams{
			Service: "remotebuildexecution.googleapis.com:443",
		},
		// We set it to slightly below 4 MiB, because that is the limit of a message size in gRPC.
		MaxBatchSize: 4*1024*1024 - 1024,
		// This is a suggested approximate limit based on current RBE implementation.
		// Above that BatchUpdateBlobs calls start to exceed a typical minute timeout.
		MaxBatchDigests:         4000,
		SmallFileThreshold:      1024 * 1024,
		BatchUploadConcurrency:  100,
		StreamUploadConcurrency: 256,
		SmallFileIOConcurrency:  64,
		Retrier:                 client.RetryTransient(),
	}
}

func (c *ClientConfig) Validate() error {
	// TODO(nodir): implement.
	return nil
}

type Client struct {
	InstanceName string
	ClientConfig
	conn       *grpc.ClientConn
	byteStream bsgrpc.ByteStreamClient
	cas        repb.ContentAddressableStorageClient
}

type Opt interface {
	Apply(*Client)
}

func NewClient(ctx context.Context, instanceName string) (*Client, error) {
	return NewClientWithConfig(ctx, instanceName, DefaultClientConfig())
}

// NewClientFromConnection creates a client from gRPC connections to a remote execution service and a cas service.
func NewClientWithConfig(ctx context.Context, instanceName string, config ClientConfig) (*Client, error) {
	if instanceName == "" {
		return nil, fmt.Errorf("instance name is unspecified")
	}

	conn, err := client.Dial(ctx, config.DialParams.Service, config.DialParams)
	if err != nil {
		return nil, err
	}

	client := &Client{
		InstanceName: instanceName,
		ClientConfig: config,
		conn:         conn,
		byteStream:   bsgrpc.NewByteStreamClient(conn),
		cas:          repb.NewContentAddressableStorageClient(conn),
	}
	// TODO(nodir): Check capabilities.
	return client, nil
}

type FilePredicate func(absName string, mode os.FileMode) bool

type UploadInput struct {
	Content []byte

	Path                string
	Predicate           FilePredicate
	IgnoreSymlinkTarget bool
	PreserveSymlinks    bool
}

type TransferStats struct {
	CacheHits   BlobStat
	CacheMisses BlobStat

	StreamingUploads BlobStat
	BatchedUploads   BlobStat
}

type BlobStat struct {
	Count int64
	Bytes int64
}

type UploadCallbackInput struct {
	Path   string
	Digest digest.Digest
}

func (c *Client) Upload(ctx context.Context, input <-chan *UploadInput) (stats *TransferStats, err error) {
	// Do not exit until all sub-goroutines exit.
	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	u := &uploader{
		Client:     c,
		eg:         eg,
		streamC:    make(chan *uploadItem, 4096),
		semBatches: semaphore.NewWeighted(int64(c.BatchUploadConcurrency)),
		semFSWalk:  semaphore.NewWeighted(int64(c.SmallFileIOConcurrency)),
	}
	u.largeFileStreamer = newStreamer(u)

	// Initialize checkBundler, which checks if a blob is present on the server.
	var wgChecks sync.WaitGroup
	u.checkBundler = bundler.NewBundler(&uploadItem{}, func(items interface{}) {
		wgChecks.Add(1)
		// Handle context cancelation and errors as a part of the error group.
		eg.Go(func() error {
			defer wgChecks.Done()
			return u.check(ctx, items.([]*uploadItem))
		})
	})
	u.checkBundler.BundleCountThreshold = c.MaxFindMissingDigestsSize

	// Initialize batchBundler, which uploads blobs in batches.
	u.batchBundler = bundler.NewBundler(&repb.BatchUpdateBlobsRequest_Request{}, func(subReq interface{}) {
		// Handle context cancelation and errors as a part of the error group.
		eg.Go(func() error {
			return u.uploadBatch(ctx, subReq.([]*repb.BatchUpdateBlobsRequest_Request))
		})
	})
	// Limit the sum of sub-request sizes to (MaxBatchSize - requestOverhead).
	u.batchBundler.BundleByteLimit = int(c.MaxBatchSize - marshalledFieldSize(int64(len(c.InstanceName))))
	u.batchBundler.BundleByteThreshold = u.batchBundler.BundleByteLimit
	u.batchBundler.BundleCountThreshold = c.MaxBatchDigests

	// Initialize streamers, which stream large blobs using ByteStream service.
	for i := 0; i < c.StreamUploadConcurrency; i++ {
		eg.Go(func() error {
			s := newStreamer(u)
			for item := range u.streamC {
				if err := s.stream(ctx, item, false); err != nil {
					return err
				}
			}
			return nil
		})
	}

	// Start processing input.
	eg.Go(func() error {
		// When exiting, flush bundlers and close streamers.
		defer func() {
			u.wgFS.Wait()
			fmt.Printf("finished fs-walk\n")

			u.checkBundler.Flush() // must be called after wgFS.Wait()
			wgChecks.Wait()        // must be called after flushing checkBundler.
			fmt.Printf("finished checks\n")

			u.batchBundler.Flush() // must be called after wgChecks.Wait()
			fmt.Printf("finished batch uploads\n")

			close(u.streamC) // must be called after wgChecks.Wait()
		}()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case in, ok := <-input:
				if !ok {
					return nil
				}
				u.eg.Go(func() error {
					return errors.Wrapf(u.process(ctx, in), "%q", in.Path)
				})
			}
		}
	})
	return &u.stats, eg.Wait()
}

func (u *uploader) process(ctx context.Context, in *UploadInput) error {
	if in.Path == "" {
		// Simple case: the blob is in memory.
		_, err := u.scheduleCheckBlob(ctx, in.Content, "")
		return err
	}

	absPath, err := filepath.Abs(in.Path)
	if err != nil {
		return err
	}
	info, err := os.Lstat(in.Path)
	if err != nil {
		return err
	}
	if in.Predicate != nil && !in.Predicate(absPath, info.Mode()) {
		return nil
	}
	_, err = u.readFile(ctx, absPath, info, in.Predicate)
	return err
}

type fsCacheKey struct {
	AbsName      string
	PredicatePtr uintptr
}

type dirNodeCacheEntry struct {
	compute  sync.Once
	dirEntry proto.Message
	err      error
}

func (u *uploader) visitRegularFile(ctx context.Context, absName string, info os.FileInfo) (*repb.FileNode, error) {
	if err := u.semFSWalk.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	defer u.semFSWalk.Release(1)

	isLarge := info.Size() > 256*1024*1024
	if isLarge {
		u.muLargeFile.Lock()
		defer u.muLargeFile.Unlock()
	}

	start := time.Now()
	f, err := os.Open(absName)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ret := &repb.FileNode{
		Name:         f.Name(),
		IsExecutable: (info.Mode() & 0100) != 0,
		Digest:       &repb.Digest{SizeBytes: info.Size()},
	}

	if info.Size() <= int64(u.SmallFileThreshold) {
		// This file is small. Buffer it entirely.
		contents, err := ioutil.ReadAll(f)
		if err != nil {
			return nil, err
		}
		item, err := u.scheduleCheckBlob(ctx, contents, absName)
		if err != nil {
			return nil, err
		}
		ret.Digest = item.digest
		return ret, nil
	}

	// It is a large file.
	h := digest.HashFn.New()
	// TODO: reuse buffer
	if _, err := io.CopyBuffer(h, f, make([]byte, 4*1024*1024)); err != nil {
		return nil, err
	}
	ret.Digest.Hash = hex.EncodeToString(h.Sum(nil))
	fmt.Printf("computed hash of %.2fMb file in %s: %s\n", float64(ret.Digest.SizeBytes)/1e6, time.Since(start), absName)

	item := &uploadItem{
		title:  absName,
		digest: ret.Digest,
	}

	if !isLarge {
		item.open = func() (readSeekCloser, error) {
			return os.Open(absName)
		}
		return ret, u.scheduleCheck(ctx, item)
	}

	item.open = func() (readSeekCloser, error) {
		_, err := f.Seek(0, io.SeekStart)
		return f, err
	}
	// Large files are special: locality is important - we want to re-read the
	// file ASAP.
	// Also we are not going to use BatchUploads anyway, so we can take advantage
	// of ByteStream's existing presence check.
	return ret, u.largeFileStreamer.stream(ctx, item, true)
}

func (u *uploader) readFile(ctx context.Context, absName string, info os.FileInfo, predicate FilePredicate) (dirEntry interface{}, err error) {
	u.wgFS.Add(1)
	defer u.wgFS.Done()

	cacheKey := fsCacheKey{AbsName: absName, PredicatePtr: reflect.ValueOf(predicate).Pointer()}
	entryRaw, _ := u.fsCache.LoadOrStore(cacheKey, &dirNodeCacheEntry{})
	e := entryRaw.(*dirNodeCacheEntry)
	e.compute.Do(func() {
		switch {
		case info.Mode()&os.ModeSymlink == os.ModeSymlink:
			e.dirEntry, e.err = u.visitSymlink(ctx, absName, predicate)
		case info.Mode().IsDir():
			e.dirEntry, e.err = u.visitDir(ctx, absName, predicate)
		case info.Mode().IsRegular():
			e.dirEntry, e.err = u.visitRegularFile(ctx, absName, info)
		default:
			e.err = fmt.Errorf("unexpected file mode %s", info.Mode())
		}
	})
	return e.dirEntry, e.err
}

func (u *uploader) visitDir(ctx context.Context, absName string, predicate FilePredicate) (*repb.DirectoryNode, error) {
	var mu sync.Mutex
	dir := &repb.Directory{}
	var subErr error
	var wg sync.WaitGroup

	err := func() error {
		if err := u.semFSWalk.Acquire(ctx, 1); err != nil {
			return err
		}
		defer u.semFSWalk.Release(1)

		f, err := os.Open(absName)
		if err != nil {
			return err
		}
		defer f.Close()

		for ctx.Err() == nil {
			// TODO: use f.ReadDir in Go 1.16
			infos, err := f.Readdir(128)
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}

			for _, info := range infos {
				info := info
				eAbsName := filepath.Join(absName, info.Name())
				if predicate != nil && !predicate(eAbsName, info.Mode()) {
					continue
				}
				wg.Add(1)
				u.eg.Go(func() error {
					defer wg.Done()
					node, err := u.readFile(ctx, eAbsName, info, predicate)

					mu.Lock()
					defer mu.Unlock()

					if err != nil {
						if subErr == nil {
							subErr = err
						}
						return err
					}

					if dir != nil {
						switch node := node.(type) {
						case *repb.FileNode:
							dir.Files = append(dir.Files, node)
						case *repb.DirectoryNode:
							dir.Directories = append(dir.Directories, node)
						case *repb.SymlinkNode:
							dir.Symlinks = append(dir.Symlinks, node)
						}
					}
					return nil
				})
			}
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}

	wg.Wait()
	if subErr != nil {
		return nil, errors.Wrapf(subErr, "failed to read the directory %q entirely", absName)
	}

	// Normalize the dir.
	sort.Slice(dir.Files, func(i, j int) bool {
		return dir.Files[i].Name < dir.Files[j].Name
	})
	sort.Slice(dir.Directories, func(i, j int) bool {
		return dir.Directories[i].Name < dir.Directories[j].Name
	})
	sort.Slice(dir.Symlinks, func(i, j int) bool {
		return dir.Symlinks[i].Name < dir.Symlinks[j].Name
	})

	marshalled, err := proto.Marshal(dir)
	if err != nil {
		return nil, err
	}
	item, err := u.scheduleCheckBlob(ctx, marshalled, absName)
	if err != nil {
		return nil, err
	}
	return &repb.DirectoryNode{
		Name:   filepath.Base(absName),
		Digest: item.digest,
	}, nil
}

func (u *uploader) visitSymlink(ctx context.Context, absName string, predicate FilePredicate) (*repb.SymlinkNode, error) {
	target, err := os.Readlink(absName)
	if err != nil {
		return nil, err
	}

	ret := &repb.SymlinkNode{
		Name:   filepath.Base(absName),
		Target: target,
	}

	absTarget := target
	if !filepath.IsAbs(absTarget) {
		absTarget = filepath.Clean(filepath.Join(filepath.Dir(absName), absTarget))
	}
	// TODO: avoid another lstat
	info, err := os.Lstat(absTarget)
	// TODO: add support for dangling links.
	if err != nil {
		return nil, err
	}
	_, err = u.readFile(ctx, absTarget, info, predicate)
	return ret, err
}

type uploader struct {
	*Client
	eg    *errgroup.Group
	stats TransferStats

	muLargeFile       sync.Mutex
	largeFileStreamer streamer

	semFSWalk *semaphore.Weighted
	wgFS      sync.WaitGroup
	fsCache   sync.Map

	checkBundler *bundler.Bundler
	seenHashes   sync.Map

	batchBundler *bundler.Bundler
	semBatches   *semaphore.Weighted

	streamC chan *uploadItem
}

type uploadItem struct {
	title  string
	digest *repb.Digest
	open   func() (readSeekCloser, error)
}

func (u *uploader) scheduleCheckBlob(ctx context.Context, data []byte, title string) (*uploadItem, error) {
	item := &uploadItem{
		title:  title,
		digest: digest.NewFromBlob(data).ToProto(),
		open: func() (readSeekCloser, error) {
			return newByteReader(data), nil
		},
	}
	if item.title == "" {
		item.title = fmt.Sprintf("blob %s", item.digest.Hash)
	}
	return item, u.scheduleCheck(ctx, item)
}

func (u *uploader) scheduleCheck(ctx context.Context, item *uploadItem) error {
	if _, ok := u.seenHashes.LoadOrStore(item.digest.Hash, struct{}{}); ok {
		return nil
	}
	return u.checkBundler.AddWait(ctx, item, 0)
}

// check checks if the items are present on the server, and schedules upload
// for the missing ones.
func (u *uploader) check(ctx context.Context, items []*uploadItem) error {
	req := &repb.FindMissingBlobsRequest{
		InstanceName: u.InstanceName,
		BlobDigests:  make([]*repb.Digest, len(items)),
	}
	byDigest := make(map[string]*uploadItem, len(items))
	totalBytes := int64(0)
	for i, item := range items {
		req.BlobDigests[i] = item.digest
		byDigest[item.digest.Hash] = item
		totalBytes += item.digest.SizeBytes
	}

	var res *repb.FindMissingBlobsResponse
	err := u.Retrier.Do(ctx, func() (err error) {
		// TODO: per-RPC timeouts.
		res, err = u.cas.FindMissingBlobs(ctx, req)
		return
	})
	if err != nil {
		return err
	}

	missingBytes := int64(0)
	for _, d := range res.MissingBlobDigests {
		missingBytes += d.SizeBytes
		if err := u.scheduleUpload(ctx, byDigest[d.Hash]); err != nil {
			return err
		}
	}
	atomic.AddInt64(&u.stats.CacheMisses.Count, int64(len(res.MissingBlobDigests)))
	atomic.AddInt64(&u.stats.CacheMisses.Bytes, missingBytes)
	atomic.AddInt64(&u.stats.CacheHits.Count, int64(len(items)-len(res.MissingBlobDigests)))
	atomic.AddInt64(&u.stats.CacheHits.Bytes, totalBytes-missingBytes)
	fmt.Printf("checked %d digests: %d missing on server\n", len(items), len(res.MissingBlobDigests))
	return nil
}

func (u *uploader) scheduleUpload(ctx context.Context, item *uploadItem) error {
	// Check if this blob can be uploaded in a batch.
	if marshalledRequestSize(item.digest) > int64(u.batchBundler.BundleByteLimit) {
		// There is no way this blob can fit in a batch request.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case u.streamC <- item:
			return nil
		}
	}

	// Since this blob is small enough, read the data.
	r, err := item.open()
	if err != nil {
		return errors.Wrapf(err, "failed to open %q", item.title)
	}
	defer r.Close()
	contents, err := ioutil.ReadAll(r)
	if err != nil {
		return errors.Wrapf(err, "failed to read %q", item.title)
	}
	if err := r.Close(); err != nil {
		return errors.Wrapf(err, "failed to close %q", item.title)
	}
	req := &repb.BatchUpdateBlobsRequest_Request{Digest: item.digest, Data: contents}
	return u.batchBundler.AddWait(ctx, req, proto.Size(req))
}

func (u *uploader) uploadBatch(ctx context.Context, reqs []*repb.BatchUpdateBlobsRequest_Request) error {
	if err := u.semBatches.Acquire(ctx, 1); err != nil {
		return err
	}
	defer u.semBatches.Release(1)

	reqMap := make(map[string]*repb.BatchUpdateBlobsRequest_Request, len(reqs))
	for _, req := range reqs {
		reqMap[req.Digest.Hash] = req
	}

	return u.Retrier.Do(ctx, func() error {
		res, err := u.cas.BatchUpdateBlobs(ctx, &repb.BatchUpdateBlobsRequest{
			InstanceName: u.InstanceName,
			Requests:     reqs,
		})
		if err != nil {
			return errors.Wrap(err, "failed to upload a batch")
		}

		var retriableError error
		reqs = reqs[:0]
		bytesTransferred := int64(0)
		digestsTransferred := int64(0)
		for _, r := range res.Responses {
			if err := status.FromProto(r.Status).Err(); err != nil {
				if !u.Retrier.ShouldRetry(err) {
					return errors.Wrapf(err, "failed to upload at least one blob %q", r.Digest.Hash)
				}
				reqs = append(reqs, reqMap[r.Digest.Hash])
				retriableError = err
				continue
			}
			bytesTransferred += r.Digest.SizeBytes
			digestsTransferred++
		}
		atomic.AddInt64(&u.stats.BatchedUploads.Bytes, bytesTransferred)
		atomic.AddInt64(&u.stats.BatchedUploads.Count, digestsTransferred)
		fmt.Printf("uploaded %.2fMB in a batch\n", float64(bytesTransferred)/1e6)
		return retriableError
	})
}

type streamer struct {
	*uploader
	diskReadBuf   []byte
	bytestreamBuf []byte
}

func newStreamer(u *uploader) streamer {
	return streamer{
		uploader:      u,
		diskReadBuf:   make([]byte, 4*1024*1024),
		bytestreamBuf: make([]byte, 32*1024),
	}
}

func (s *streamer) stream(ctx context.Context, item *uploadItem, largeFileMode bool) error {
	fmt.Printf("starting to stream %s, %d bytes\n", item.title, item.digest.SizeBytes)

	r, err := item.open()
	if err != nil {
		return err
	}

	// zstd := zstdEncoders.Get().(*zstd.Encoder)
	// defer zstdEncoders.Put(zstd)

	rewind := false
	return s.Retrier.Do(ctx, func() error {
		if rewind {
			if _, err := r.Seek(0, io.SeekStart); err != nil {
				return err
			}
		}
		rewind = true

		start := time.Now()

		stream, err := s.byteStream.Write(ctx)
		if err != nil {
			return err
		}

		// req := &bspb.WriteRequest{
		// 	ResourceName: fmt.Sprintf("%s/uploads/%s/compressed-blobs/zstd/%s/%d", s.InstanceName, uuid.New(), item.digest.Hash, item.digest.SizeBytes),
		// }
		// // TODO: this could be more optimized: reads from fs and writes to cas
		// // block each other. Could be concurrent.
		// // Could be solved by enabling readahead, or by making this code
		// for {
		// 	// Before reading, check if the context if canceled.
		// 	if ctx.Err() != nil {
		// 		return ctx.Err()
		// 	}

		// 	// Read the next chunk.
		// 	n, err := io.ReadFull(r, s.buf)
		// 	switch {
		// 	case err == io.EOF || err == io.ErrUnexpectedEOF:
		// 		req.FinishWrite = true
		// 	case err != nil:
		// 		return err
		// 	}
		// 	req.Data = zstdEncoder.EncodeAll(s.buf[:n], req.Data[:0])

		// 	// Send the chunk and handle the response.
		// 	err = stream.Send(req)
		// 	if err == io.EOF {
		// 		break
		// 	}
		// 	if err != nil {
		// 		return err
		// 	}
		// 	if req.FinishWrite {
		// 		break
		// 	}
		// 	req.ResourceName = ""
		// 	req.WriteOffset += int64(n)
		// }
		// switch res, err := stream.CloseAndRecv(); {
		// case err != nil:
		// 	return err
		// case res.CommittedSize != item.digest.SizeBytes:
		// 	return fmt.Errorf("unexpected commitSize: got %d, want %d", res.CommittedSize, item.digest.SizeBytes)
		// }

		// fmt.Printf("streamed %.2fMB in %s: %s\n", float64(req.WriteOffset)/1e6, time.Since(start), item.title)
		// atomic.AddInt64(&s.stats.StreamingUploads.Bytes, req.WriteOffset)
		// atomic.AddInt64(&s.stats.StreamingUploads.Count, 1)
		// if largeFileMode {
		// 	st := &s.stats.CacheHits
		// 	if req.FinishWrite {
		// 		st = &s.stats.CacheMisses
		// 	}
		// 	atomic.AddInt64(&st.Bytes, item.digest.SizeBytes)
		// 	atomic.AddInt64(&st.Count, 1)
		// }

		// return nil

		zstd := zstdEncoders.Get().(*zstd.Encoder)
		defer zstdEncoders.Put(zstd)
		pr, pw := io.Pipe()
		zstd.Reset(pw)

		eg, ctx := errgroup.WithContext(ctx)
		eg.Go(func() error {
			_, err := io.CopyBuffer(zstd, r, s.diskReadBuf)
			if err == io.ErrClosedPipe {
				// The other goroutine exited before we finished encoding.
				// Must be a cache hit.
				return nil
			}
			if err != nil {
				return err
			}

			if err := zstd.Close(); err != nil {
				return err
			}

			return pw.Close()
		})
		eg.Go(func() error {
			defer pr.Close()
			req := &bsgrpc.WriteRequest{
				ResourceName: fmt.Sprintf("%s/uploads/%s/compressed-blobs/zstd/%s/%d", s.InstanceName, uuid.New(), item.digest.Hash, item.digest.SizeBytes),
			}
			for {
				// Before reading, check if the context if canceled.
				if ctx.Err() != nil {
					return ctx.Err()
				}

				// Read the next chunk.
				n, err := io.ReadFull(pr, s.bytestreamBuf)
				switch {
				case err == io.EOF || err == io.ErrUnexpectedEOF:
					req.FinishWrite = true
				case err != nil:
					return err
				}
				req.Data = s.bytestreamBuf[:n]

				// Send the chunk and handle the response.
				err = stream.Send(req)
				if err == io.EOF {
					break
				}
				if err != nil {
					return err
				}
				if req.FinishWrite {
					break
				}
				req.ResourceName = ""
				req.WriteOffset += int64(len(req.Data))
			}
			switch res, err := stream.CloseAndRecv(); {
			case err != nil:
				fmt.Println(err)
				return err
			case res.CommittedSize != item.digest.SizeBytes:
				return fmt.Errorf("unexpected commitSize: got %d, want %d", res.CommittedSize, item.digest.SizeBytes)
			}

			fmt.Printf("streamed %.2fMB in %s: %s\n", float64(req.WriteOffset)/1e6, time.Since(start), item.title)
			atomic.AddInt64(&s.stats.StreamingUploads.Bytes, req.WriteOffset)
			atomic.AddInt64(&s.stats.StreamingUploads.Count, 1)
			if largeFileMode {
				st := &s.stats.CacheHits
				if req.FinishWrite {
					st = &s.stats.CacheMisses
				}
				atomic.AddInt64(&st.Bytes, item.digest.SizeBytes)
				atomic.AddInt64(&st.Count, 1)
			}

			return nil
		})
		return eg.Wait()
	})
}

type byteReader struct {
	content []byte
	r       io.Reader
}

func newByteReader(content []byte) *byteReader {
	return &byteReader{content: content, r: bytes.NewReader(content)}
}

func (r *byteReader) Read(p []byte) (int, error) {
	return r.r.Read(p)
}

func (r *byteReader) Seek(offset int64, whence int) (int64, error) {
	if offset != 0 || whence != io.SeekCurrent {
		return 0, fmt.Errorf("unsupported")
	}
	r.r = bytes.NewReader(r.content)
	return 0, nil
}

func (r *byteReader) Close() error {
	return nil
}

func marshalledFieldSize(size int64) int64 {
	return 1 + int64(proto.SizeVarint(uint64(size))) + size
}

func marshalledRequestSize(d *repb.Digest) int64 {
	// An additional BatchUpdateBlobsRequest_Request includes the Digest and data fields,
	// as well as the message itself. Every field has a 1-byte size tag, followed by
	// the varint field size for variable-sized fields (digest hash and data).
	// Note that the BatchReadBlobsResponse_Response field is similar, but includes
	// and additional Status proto which can theoretically be unlimited in size.
	// We do not account for it here, relying on the Client setting a large (100MB)
	// limit for incoming messages.
	digestSize := marshalledFieldSize(int64(len(d.Hash)))
	if d.SizeBytes > 0 {
		digestSize += 1 + int64(proto.SizeVarint(uint64(d.SizeBytes)))
	}
	reqSize := marshalledFieldSize(digestSize)
	if d.SizeBytes > 0 {
		reqSize += marshalledFieldSize(int64(d.SizeBytes))
	}
	return marshalledFieldSize(reqSize)
}

type readSeekCloser interface {
	io.ReadSeeker
	io.Closer
}
