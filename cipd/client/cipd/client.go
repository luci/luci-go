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

// Package cipd implements client side of Chrome Infra Package Deployer.
//
// Binary package file format (in free form representation):
//   <binary package> := <zipped data>
//   <zipped data> := DeterministicZip(<all input files> + <manifest json>)
//   <manifest json> := File{
//     name: ".cipdpkg/manifest.json",
//     data: JSON({
//       "FormatVersion": "1",
//       "PackageName": <name of the package>
//     }),
//   }
//   DeterministicZip = zip archive with deterministic ordering of files and stripped timestamps
//
// Main package data (<zipped data> above) is deterministic, meaning its content
// depends only on inputs used to built it (byte to byte): contents and names of
// all files added to the package (plus 'executable' file mode bit) and
// a package name (and all other data in the manifest).
//
// Binary package data MUST NOT depend on a timestamp, hostname of machine that
// built it, revision of the source code it was built from, etc. All that
// information will be distributed as a separate metadata packet associated with
// the package when it gets uploaded to the server.
//
// TODO: expand more when there's server-side package data model (labels
// and stuff).
package cipd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/prpc"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/client/cipd/internal"
	"go.chromium.org/luci/cipd/client/cipd/local"
	"go.chromium.org/luci/cipd/client/cipd/platform"
	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/version"
)

const (
	// CASFinalizationTimeout is how long to wait for CAS service to finalize
	// the upload in RegisterInstance.
	CASFinalizationTimeout = 5 * time.Minute

	// SetRefTimeout is how long to wait for an instance to be processed when
	// setting a ref in SetRefWhenReady.
	SetRefTimeout = 3 * time.Minute

	// TagAttachTimeout is how long to wait for an instance to be processed when
	// attaching tags in AttachTagsWhenReady.
	TagAttachTimeout = 3 * time.Minute
)

// Environment variable definitions
const (
	EnvCacheDir            = "CIPD_CACHE_DIR"
	EnvHTTPUserAgentPrefix = "CIPD_HTTP_USER_AGENT_PREFIX"
)

var (
	// ErrFinalizationTimeout is returned if CAS service can not finalize upload
	// fast enough.
	ErrFinalizationTimeout = errors.New("timeout while waiting for CAS service to finalize the upload", transient.Tag)

	// ErrBadUpload is returned when a package file is uploaded, but servers asks
	// us to upload it again.
	ErrBadUpload = errors.New("package file is uploaded, but servers asks us to upload it again", transient.Tag)

	// ErrProcessingTimeout is returned by SetRefWhenReady or AttachTagsWhenReady
	// if the instance processing on the backend takes longer than expected. Refs
	// and tags can be attached only to processed instances.
	ErrProcessingTimeout = errors.New("timeout while waiting for the instance to become ready", transient.Tag)

	// ErrDownloadError is returned by FetchInstance on download errors.
	ErrDownloadError = errors.New("failed to download the package file after multiple attempts", transient.Tag)

	// ErrUploadError is returned by RegisterInstance on upload errors.
	ErrUploadError = errors.New("failed to upload the package file after multiple attempts", transient.Tag)

	// ErrEnsurePackagesFailed is returned by EnsurePackages if something is not
	// right.
	ErrEnsurePackagesFailed = errors.New("failed to update packages, see the log")
)

// ParanoidMode specifies how paranoid EnsurePackages should be.
type ParanoidMode = common.ParanoidMode

const (
	// NotParanoid indicates that EnsurePackages should trust its metadata
	// directory: if a package is marked as installed there, it should be
	// considered correctly installed in the site root too.
	NotParanoid = common.NotParanoid

	// CheckPresence indicates that CheckDeployed should verify all files
	// that are supposed to be installed into the site root are indeed present
	// there, and reinstall ones that are missing.
	//
	// Note that it will not check file's content or file mode. Only its presence.
	CheckPresence = common.CheckPresence
)

var (
	// ClientPackage is a package with the CIPD client. Used during self-update.
	ClientPackage = "infra/tools/cipd/${platform}"
	// UserAgent is HTTP user agent string for CIPD client.
	UserAgent = "cipd 2.1.0"
)

func init() {
	ver, err := version.GetStartupVersion()
	if err != nil || ver.InstanceID == "" {
		return
	}
	UserAgent += fmt.Sprintf(" (%s@%s)", ver.PackageName, ver.InstanceID)
}

// UploadSession describes open CAS upload session.
type UploadSession struct {
	// ID identifies upload session in the backend.
	ID string
	// URL is where to upload the data to.
	URL string
}

// DescribeInstanceOpts is passed to DescribeInstance.
type DescribeInstanceOpts struct {
	DescribeRefs bool // if true, will fetch all refs pointing to the instance
	DescribeTags bool // if true, will fetch all tags attached to the instance
}

// Client provides high-level CIPD client interface. Thread safe.
type Client interface {
	// BeginBatch makes the client enter into a "batch mode".
	//
	// In this mode various cleanup and cache updates, usually performed right
	// away, are deferred until 'EndBatch' call.
	//
	// This is an optimization. Use it if you plan to call a bunch of Client
	// methods in a short amount of time (parallel or sequentially).
	//
	// Batches can be nested.
	BeginBatch(ctx context.Context)

	// EndBatch ends a batch started with BeginBatch.
	//
	// EndBatch does various delayed maintenance tasks (like cache updates, trash
	// cleanup and so on). This is best-effort operations, and thus this method
	// doesn't return an errors.
	//
	// See also BeginBatch doc for more details.
	EndBatch(ctx context.Context)

	// FetchACL returns a list of PackageACL objects (parent paths first).
	//
	// Together they define the access control list for the given package prefix.
	FetchACL(ctx context.Context, prefix string) ([]PackageACL, error)

	// ModifyACL applies a set of PackageACLChanges to a package prefix ACL.
	ModifyACL(ctx context.Context, prefix string, changes []PackageACLChange) error

	// FetchRoles returns all roles the caller has in the given package prefix.
	//
	// Understands roles inheritance, e.g. if the caller is OWNER, the return
	// value will list all roles implied by being an OWNER (e.g. READER, WRITER,
	// ...).
	FetchRoles(ctx context.Context, prefix string) ([]string, error)

	// ResolveVersion converts an instance ID, a tag or a ref into a concrete Pin.
	ResolveVersion(ctx context.Context, packageName, version string) (common.Pin, error)

	// RegisterInstance makes the package instance available for clients.
	//
	// It uploads the instance to the storage, waits until the storage verifies
	// its hash, and then registers the package in the repository, making it
	// discoverable.
	//
	// 'timeout' specifies for how long to wait until the instance hash is
	// verified by the storage backend. If 0, default CASFinalizationTimeout will
	// be used.
	RegisterInstance(ctx context.Context, instance local.PackageInstance, timeout time.Duration) error

	// DescribeInstance returns information about a package instance.
	//
	// May also be used as a simple instance presence check, if opts is nil. If
	// the request succeeds, then the instance exists.
	DescribeInstance(ctx context.Context, pin common.Pin, opts *DescribeInstanceOpts) (*InstanceDescription, error)

	// SetRefWhenReady moves a ref to point to a package instance.
	SetRefWhenReady(ctx context.Context, ref string, pin common.Pin) error

	// AttachTagsWhenReady attaches tags to an instance.
	AttachTagsWhenReady(ctx context.Context, pin common.Pin, tags []string) error

	// FetchPackageRefs returns information about all refs defined for a package.
	//
	// The returned list is sorted by modification timestamp (newest first).
	FetchPackageRefs(ctx context.Context, packageName string) ([]RefInfo, error)

	// FetchInstance downloads a package instance file from the repository.
	//
	// It verifies that the package hash matches pin.InstanceID.
	//
	// It returns an InstanceFile pointing to the raw package data. The caller
	// must close it when done.
	FetchInstance(ctx context.Context, pin common.Pin) (local.InstanceFile, error)

	// FetchInstanceTo downloads a package instance file into the given writer.
	//
	// This is roughly the same as getting a reader with 'FetchInstance' and
	// copying its data into the writer, except this call skips unnecessary temp
	// files if the client is not using cache.
	//
	// It verifies that the package hash matches pin.InstanceID, but does it while
	// writing to 'output', so expect to discard all data there if FetchInstanceTo
	// returns an error.
	FetchInstanceTo(ctx context.Context, pin common.Pin, output io.WriteSeeker) error

	// FetchAndDeployInstance fetches the package instance and deploys it.
	//
	// Deploys to the given subdir under the site root (see ClientOptions.Root).
	// It doesn't check whether the instance is already deployed.
	FetchAndDeployInstance(ctx context.Context, subdir string, pin common.Pin) error

	// ListPackages returns a list packages and prefixes under the given prefix.
	ListPackages(ctx context.Context, prefix string, recursive, includeHidden bool) ([]string, error)

	// SearchInstances finds instances of some package with all given tags.
	//
	// Returns their concrete Pins.
	SearchInstances(ctx context.Context, packageName string, tags []string) (common.PinSlice, error)

	// ListInstances enumerates instances of a package, most recent first.
	//
	// Returns an object that can be used to fetch the listing, page by page.
	ListInstances(ctx context.Context, packageName string) (InstanceEnumerator, error)

	// EnsurePackages installs, removes and updates packages in the site root.
	//
	// Given a description of what packages (and versions) should be installed it
	// will do all necessary actions to bring the state of the site root to the
	// desired one.
	//
	// Depending on the paranoia mode, will optionally verify that all installed
	// packages are installed correctly and will attempt to fix ones that are not.
	// See the enum for more info.
	//
	// If dryRun is true, will just check for changes and return them in Actions
	// struct, but won't actually perform them.
	//
	// If the update was only partially applied, returns both Actions and error.
	EnsurePackages(ctx context.Context, pkgs common.PinSliceBySubdir, paranoia ParanoidMode, dryRun bool) (ActionMap, error)
}

// ClientOptions is passed to NewClient factory function.
type ClientOptions struct {
	// ServiceURL is root URL of the backend service.
	//
	// Default is ServiceURL const.
	ServiceURL string

	// Root is a site root directory.
	//
	// It is a directory where packages will be installed to. It also hosts
	// .cipd/* directory that tracks internal state of installed packages and
	// keeps various cache files. 'Root' can be an empty string if the client is
	// not going to be used to deploy or remove local packages.
	Root string

	// CacheDir is a directory for shared cache.
	//
	// If empty, instances are not cached and tags are cached inside the site
	// root. If both Root and CacheDir are empty, tag cache is disabled.
	CacheDir string

	// AnonymousClient is http.Client that doesn't attach authentication headers.
	//
	// Will be used when talking to the Google Storage. We use signed URLs that do
	// not require additional authentication.
	//
	// Default is http.DefaultClient.
	AnonymousClient *http.Client

	// AuthenticatedClient is http.Client that attaches authentication headers.
	//
	// Will be used when talking to the backend.
	//
	// Default is same as AnonymousClient (it will probably not work for most
	// packages, since the backend won't authorize an anonymous access).
	AuthenticatedClient *http.Client

	// UserAgent is put into User-Agent HTTP header with each request.
	//
	// Default is UserAgent const.
	UserAgent string

	// Mocks used by tests.
	casMock     api.StorageClient
	repoMock    api.RepositoryClient
	storageMock storage
}

// LoadFromEnv loads supplied default values from an environment into opts.
//
// The supplied getEnv function is used to access named enviornment variables,
// and should return an empty string if the enviornment variable is not defined.
func (opts *ClientOptions) LoadFromEnv(getEnv func(string) string) error {
	if opts.CacheDir == "" {
		if v := getEnv(EnvCacheDir); v != "" {
			if !filepath.IsAbs(v) {
				return fmt.Errorf("bad %s: not an absolute path - %s", EnvCacheDir, v)
			}
			opts.CacheDir = v
		}
	}
	if opts.UserAgent == "" {
		if v := getEnv(EnvHTTPUserAgentPrefix); v != "" {
			opts.UserAgent = fmt.Sprintf("%s/%s", v, UserAgent)
		}
	}
	return nil
}

// NewClient initializes CIPD client object.
func NewClient(opts ClientOptions) (Client, error) {
	if opts.AnonymousClient == nil {
		opts.AnonymousClient = http.DefaultClient
	}
	if opts.AuthenticatedClient == nil {
		opts.AuthenticatedClient = opts.AnonymousClient
	}
	if opts.UserAgent == "" {
		opts.UserAgent = UserAgent
	}

	// Validate and normalize service URL.
	if opts.ServiceURL == "" {
		return nil, fmt.Errorf("ServiceURL is required")
	}
	parsed, err := url.Parse(opts.ServiceURL)
	if err != nil {
		return nil, fmt.Errorf("not a valid URL %q - %s", opts.ServiceURL, err)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return nil, fmt.Errorf("expecting a root URL, not %q", opts.ServiceURL)
	}
	opts.ServiceURL = fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)

	prpcC := &prpc.Client{
		C:    opts.AuthenticatedClient,
		Host: parsed.Host,
		Options: &prpc.Options{
			UserAgent: opts.UserAgent,
			Insecure:  parsed.Scheme == "http", // for testing with local dev server
			Retry: func() retry.Iterator {
				return &retry.ExponentialBackoff{
					Limited: retry.Limited{
						Delay:   time.Second,
						Retries: 10,
					},
				}
			},
		},
	}

	cas := opts.casMock
	if cas == nil {
		cas = api.NewStoragePRPCClient(prpcC)
	}
	repo := opts.repoMock
	if repo == nil {
		repo = api.NewRepositoryPRPCClient(prpcC)
	}
	storage := opts.storageMock
	if storage == nil {
		storage = &storageImpl{
			chunkSize: uploadChunkSize,
			userAgent: opts.UserAgent,
			client:    opts.AnonymousClient,
		}
	}

	return &clientImpl{
		ClientOptions: opts,
		cas:           cas,
		repo:          repo,
		storage:       storage,
		deployer:      local.NewDeployer(opts.Root),
	}, nil
}

// MaybeUpdateClient will update the client binary at clientExe (given as
// a native path) to targetVersion if it's out of date (based on its hash).
//
// This update is done from the "infra/tools/cipd/${platform}" package, see
// ClientPackage. The function will use the given ClientOptions to figure out
// how to establish a connection with the backend. Its Root and CacheDir values
// are ignored (values derived from clientExe are used instead).
//
// Note that this function make sense only in a context of a default CIPD CLI
// client. Other binaries that link to cipd package should not use it, they'll
// be "updated" to the CIPD client binary.
func MaybeUpdateClient(ctx context.Context, opts ClientOptions, targetVersion, clientExe string) (common.Pin, error) {
	if err := common.ValidateInstanceVersion(targetVersion); err != nil {
		return common.Pin{}, err
	}

	opts.Root = filepath.Dir(clientExe)
	opts.CacheDir = filepath.Join(opts.Root, ".cipd_client_cache")

	client, err := NewClient(opts)
	if err != nil {
		return common.Pin{}, err
	}
	impl := client.(*clientImpl)

	fs := local.NewFileSystem(opts.Root, filepath.Join(opts.CacheDir, "trash"))
	defer fs.CleanupTrash(ctx)

	pin, err := impl.maybeUpdateClient(ctx, fs, targetVersion, clientExe)
	if err == nil {
		impl.ensureClientVersionInfo(ctx, fs, pin, clientExe)
	}
	return pin, err
}

type clientImpl struct {
	ClientOptions

	// pRPC API clients.
	cas  api.StorageClient
	repo api.RepositoryClient

	// batchLock protects guts of by BeginBatch/EndBatch implementation.
	batchLock    sync.Mutex
	batchNesting int
	batchPending map[batchAwareOp]struct{}

	// storage knows how to upload and download raw binaries using signed URLs.
	storage storage
	// deployer knows how to install packages to local file system. Thread safe.
	deployer local.Deployer

	// tagCache is a file-system based cache of resolved tags.
	tagCache     *internal.TagCache
	tagCacheInit sync.Once

	// instanceCache is a file-system based cache of instances.
	instanceCache     *internal.InstanceCache
	instanceCacheInit sync.Once
}

type batchAwareOp int

const (
	batchAwareOpSaveTagCache batchAwareOp = iota
	batchAwareOpCleanupTrash
)

// See https://golang.org/ref/spec#Method_expressions
var batchAwareOps = map[batchAwareOp]func(*clientImpl, context.Context){
	batchAwareOpSaveTagCache: (*clientImpl).saveTagCache,
	batchAwareOpCleanupTrash: (*clientImpl).cleanupTrash,
}

func (client *clientImpl) saveTagCache(ctx context.Context) {
	if client.tagCache != nil {
		if err := client.tagCache.Save(ctx); err != nil {
			logging.Warningf(ctx, "cipd: failed to save tag cache - %s", err)
		}
	}
}

func (client *clientImpl) cleanupTrash(ctx context.Context) {
	if err := client.deployer.CleanupTrash(ctx); err != nil {
		logging.Warningf(ctx, "cipd: failed to cleanup trash (this is fine) - %s", err)
	}
}

// getTagCache lazy-initializes tagCache and returns it.
//
// May return nil if tag cache is disabled.
func (client *clientImpl) getTagCache() *internal.TagCache {
	client.tagCacheInit.Do(func() {
		var dir string
		switch {
		case client.CacheDir != "":
			dir = client.CacheDir
		case client.Root != "":
			dir = filepath.Join(client.Root, local.SiteServiceDir)
		default:
			return
		}
		parsed, err := url.Parse(client.ServiceURL)
		if err != nil {
			panic(err) // the URL has been validated in NewClient already
		}
		client.tagCache = internal.NewTagCache(local.NewFileSystem(dir, ""), parsed.Host)
	})
	return client.tagCache
}

// getInstanceCache lazy-initializes instanceCache and returns it.
//
// May return nil if instance cache is disabled.
func (client *clientImpl) getInstanceCache(ctx context.Context) *internal.InstanceCache {
	client.instanceCacheInit.Do(func() {
		if client.CacheDir == "" {
			return
		}
		path := filepath.Join(client.CacheDir, "instances")
		client.instanceCache = internal.NewInstanceCache(local.NewFileSystem(path, ""))
		logging.Infof(ctx, "cipd: using instance cache at %q", path)
	})
	return client.instanceCache
}

func (client *clientImpl) BeginBatch(ctx context.Context) {
	client.batchLock.Lock()
	defer client.batchLock.Unlock()
	client.batchNesting++
}

func (client *clientImpl) EndBatch(ctx context.Context) {
	client.batchLock.Lock()
	defer client.batchLock.Unlock()
	if client.batchNesting <= 0 {
		panic("EndBatch called without corresponding BeginBatch")
	}
	client.batchNesting--
	if client.batchNesting == 0 {
		// Execute all pending batch aware calls now.
		for op := range client.batchPending {
			batchAwareOps[op](client, ctx)
		}
		client.batchPending = nil
	}
}

func (client *clientImpl) doBatchAwareOp(ctx context.Context, op batchAwareOp) {
	client.batchLock.Lock()
	defer client.batchLock.Unlock()
	if client.batchNesting == 0 {
		// Not inside a batch, execute right now.
		batchAwareOps[op](client, ctx)
	} else {
		// Schedule to execute when 'EndBatch' is called.
		if client.batchPending == nil {
			client.batchPending = make(map[batchAwareOp]struct{}, 1)
		}
		client.batchPending[op] = struct{}{}
	}
}

func (client *clientImpl) FetchACL(ctx context.Context, prefix string) ([]PackageACL, error) {
	if _, err := common.ValidatePackagePrefix(prefix); err != nil {
		return nil, err
	}

	resp, err := client.repo.GetInheritedPrefixMetadata(ctx, &api.PrefixRequest{
		Prefix: prefix,
	}, expectedCodes)
	if err != nil {
		return nil, humanErr(err)
	}

	return prefixMetadataToACLs(resp), nil
}

func (client *clientImpl) ModifyACL(ctx context.Context, prefix string, changes []PackageACLChange) error {
	if _, err := common.ValidatePackagePrefix(prefix); err != nil {
		return err
	}

	// Fetch existing metadata, if any.
	meta, err := client.repo.GetPrefixMetadata(ctx, &api.PrefixRequest{
		Prefix: prefix,
	}, expectedCodes)
	if code := grpc.Code(err); code != codes.OK && code != codes.NotFound {
		return humanErr(err)
	}

	// Construct new empty metadata for codes.NotFound.
	if meta == nil {
		meta = &api.PrefixMetadata{Prefix: prefix}
	}

	// Apply mutations.
	if dirty, err := mutateACLs(meta, changes); !dirty || err != nil {
		return err
	}

	// Store the new metadata. This call will check meta.Fingerprint.
	_, err = client.repo.UpdatePrefixMetadata(ctx, meta, expectedCodes)
	return humanErr(err)
}

func (client *clientImpl) FetchRoles(ctx context.Context, prefix string) ([]string, error) {
	if _, err := common.ValidatePackagePrefix(prefix); err != nil {
		return nil, err
	}

	resp, err := client.repo.GetRolesInPrefix(ctx, &api.PrefixRequest{
		Prefix: prefix,
	}, expectedCodes)
	if err != nil {
		return nil, humanErr(err)
	}

	out := make([]string, len(resp.Roles))
	for i, r := range resp.Roles {
		out[i] = r.Role.String()
	}
	return out, nil
}

func (client *clientImpl) ListPackages(ctx context.Context, prefix string, recursive, includeHidden bool) ([]string, error) {
	if _, err := common.ValidatePackagePrefix(prefix); err != nil {
		return nil, err
	}

	resp, err := client.repo.ListPrefix(ctx, &api.ListPrefixRequest{
		Prefix:        prefix,
		Recursive:     recursive,
		IncludeHidden: includeHidden,
	}, expectedCodes)
	if err != nil {
		return nil, humanErr(err)
	}

	listing := resp.Packages
	for _, pfx := range resp.Prefixes {
		listing = append(listing, pfx+"/")
	}
	sort.Strings(listing)
	return listing, nil
}

func (client *clientImpl) ResolveVersion(ctx context.Context, packageName, version string) (common.Pin, error) {
	if err := common.ValidatePackageName(packageName); err != nil {
		return common.Pin{}, err
	}

	// Is it instance ID already? Don't bother calling the backend.
	if common.ValidateInstanceID(version) == nil {
		return common.Pin{PackageName: packageName, InstanceID: version}, nil
	}
	if err := common.ValidateInstanceVersion(version); err != nil {
		return common.Pin{}, err
	}

	// Use a local cache when resolving tags to avoid round trips to the backend
	// when calling same 'cipd ensure' command again and again.
	var cache *internal.TagCache
	if common.ValidateInstanceTag(version) == nil {
		cache = client.getTagCache() // note: may be nil if the cache is disabled
	}
	if cache != nil {
		cached, err := cache.ResolveTag(ctx, packageName, version)
		if err != nil {
			logging.Warningf(ctx, "cipd: could not query tag cache - %s", err)
		}
		if cached.InstanceID != "" {
			logging.Debugf(ctx, "cipd: tag cache hit for %s:%s - %s", packageName, version, cached.InstanceID)
			return cached, nil
		}
	}

	// Either resolving a ref, or a tag cache miss? Hit the backend.
	resp, err := client.repo.ResolveVersion(ctx, &api.ResolveVersionRequest{
		Package: packageName,
		Version: version,
	}, expectedCodes)
	if err != nil {
		return common.Pin{}, humanErr(err)
	}
	pin := common.Pin{
		PackageName: packageName,
		InstanceID:  common.ObjectRefToInstanceID(resp.Instance),
	}

	// If was resolving a tag, store it in the cache.
	if cache != nil {
		if err := cache.AddTag(ctx, pin, version); err != nil {
			logging.Warningf(ctx, "cipd: could not add tag to the cache")
		}
		client.doBatchAwareOp(ctx, batchAwareOpSaveTagCache)
	}

	return pin, nil
}

// ensureClientVersionInfo is called only with the specially constructed client,
// see MaybeUpdateClient function.
func (client *clientImpl) ensureClientVersionInfo(ctx context.Context, fs local.FileSystem, pin common.Pin, clientExe string) {
	expect, err := json.Marshal(version.Info{
		PackageName: pin.PackageName,
		InstanceID:  pin.InstanceID,
	})
	if err != nil {
		// Should never occur; only error could be if version.Info is not JSON
		// serializable.
		logging.WithError(err).Errorf(ctx, "Unable to generate version file content")
		return
	}

	verFile := version.GetVersionFile(clientExe)
	if data, err := ioutil.ReadFile(verFile); err == nil && bytes.Equal(expect, data) {
		return // up to date
	}

	// There was an error reading the existing version file, or its content does
	// not match. Proceed with EnsureFile.
	err = fs.EnsureFile(ctx, verFile, func(of *os.File) error {
		_, err := of.Write(expect)
		return err
	})
	if err != nil {
		logging.WithError(err).Warningf(ctx, "Unable to update version info %q", verFile)
	}
}

// maybeUpdateClient is called only with the specially constructed client, see
// MaybeUpdateClient function.
func (client *clientImpl) maybeUpdateClient(ctx context.Context, fs local.FileSystem, targetVersion, clientExe string) (pin common.Pin, err error) {
	// currentHashMatches calculates (with memoization) the existing client binary
	// hash and compares it to 'obj'.
	var lastCalculated *api.ObjectRef
	currentHashMatches := func(obj *api.ObjectRef) (yep bool, err error) {
		if lastCalculated != nil && lastCalculated.HashAlgo == obj.HashAlgo {
			return lastCalculated.HexDigest == obj.HexDigest, nil
		}
		hash, err := common.NewHash(obj.HashAlgo)
		if err != nil {
			return false, err
		}
		file, err := os.Open(clientExe)
		if err != nil {
			return false, err
		}
		defer file.Close()
		if _, err := io.Copy(hash, file); err != nil {
			return false, err
		}
		lastCalculated = &api.ObjectRef{
			HashAlgo:  obj.HashAlgo,
			HexDigest: common.HexDigest(hash),
		}
		return lastCalculated.HexDigest == obj.HexDigest, nil
	}

	client.BeginBatch(ctx)
	defer client.EndBatch(ctx)

	clientPackage, err := template.DefaultExpander().Expand(ClientPackage)
	if err != nil {
		return // shouldn't be happening in reality
	}
	if pin, err = client.ResolveVersion(ctx, clientPackage, targetVersion); err != nil {
		return
	}

	clientFileName := "cipd"
	if platform.CurrentOS() == "windows" {
		clientFileName = "cipd.exe"
	}

	// Get the expected hash (in a form of ObjectRef) of the client binary
	// corresponding to the resolved pin. See AddExtractedObjectRef call below for
	// where it is stored.
	var exeObj *api.ObjectRef
	cache := client.getTagCache()
	if cache != nil {
		if exeObj, err = cache.ResolveExtractedObjectRef(ctx, pin, clientFileName); err != nil {
			return
		}
	}

	// Compare the hash of the running binary to the one that matches 'pin' (if
	// we know it already from the cache).
	if exeObj != nil {
		var yep bool
		if yep, err = currentHashMatches(exeObj); err != nil || yep {
			return // either can't read clientExe, or already up-to-date
		}
	}

	if targetVersion == pin.InstanceID {
		logging.Infof(ctx, "cipd: updating client to %s", pin)
	} else {
		logging.Infof(ctx, "cipd: updating client to %s (%s)", pin, targetVersion)
	}

	// Grab the signed URL to the client binary and a list of hex digests
	// calculated using all hash algos known to the server.
	info, err := client.repo.DescribeClient(ctx, &api.DescribeClientRequest{
		Package:  pin.PackageName,
		Instance: common.InstanceIDToObjectRef(pin.InstanceID),
	}, expectedCodes)
	if err != nil {
		return common.Pin{}, humanErr(err)
	}

	// Fallback value if the server doesn't support ClientRefAliases yet.
	clientRef := &api.ObjectRef{
		HashAlgo:  api.HashAlgo_SHA1,
		HexDigest: info.LegacySha1,
	}

	// Pick the best hash algo we understand to use for verification (we assume
	// the higher the hash enum value, the better the hash algo).
	for _, ref := range info.ClientRefAliases {
		if ref.HashAlgo > clientRef.HashAlgo && api.HashAlgo_name[int32(ref.HashAlgo)] != "" {
			clientRef = ref
		}
	}

	// Store the mapping 'resolve pin => hash of the client binary'.
	if cache != nil {
		err = cache.AddExtractedObjectRef(ctx, pin, clientFileName, clientRef)
		if err != nil {
			return
		}
		client.doBatchAwareOp(ctx, batchAwareOpSaveTagCache)
	}

	var yep bool
	if yep, err = currentHashMatches(clientRef); err != nil || yep {
		// Either can't read clientExe, or already up-to-date, but the cache didn't
		// know that. It knows now.
		return
	}

	// The running client is actually stale. Replace it.
	err = client.installClient(
		ctx, fs,
		common.MustNewHash(clientRef.HashAlgo),
		info.ClientBinary.SignedUrl,
		clientExe,
		clientRef.HexDigest)
	return
}

func (client *clientImpl) RegisterInstance(ctx context.Context, instance local.PackageInstance, timeout time.Duration) error {
	if timeout == 0 {
		timeout = CASFinalizationTimeout
	}
	pin := instance.Pin()

	// attemptToRegister calls RegisterInstance RPC and logs the result.
	attemptToRegister := func() (*api.UploadOperation, error) {
		logging.Infof(ctx, "cipd: registering %s", pin)
		resp, err := client.repo.RegisterInstance(ctx, &api.Instance{
			Package:  pin.PackageName,
			Instance: common.InstanceIDToObjectRef(pin.InstanceID),
		}, expectedCodes)
		if err != nil {
			return nil, humanErr(err)
		}
		switch resp.Status {
		case api.RegistrationStatus_REGISTERED:
			logging.Infof(ctx, "cipd: instance %s was successfully registered", pin)
			return nil, nil
		case api.RegistrationStatus_ALREADY_REGISTERED:
			logging.Infof(
				ctx, "cipd: instance %s is already registered by %s on %s",
				pin, resp.Instance.RegisteredBy,
				google.TimeFromProto(resp.Instance.RegisteredTs).Local())
			return nil, nil
		case api.RegistrationStatus_NOT_UPLOADED:
			return resp.UploadOp, nil
		default:
			return nil, fmt.Errorf("unrecognized package registration status %s", resp.Status)
		}
	}

	// Attempt to register. May be asked to actually upload the file first.
	uploadOp, err := attemptToRegister()
	switch {
	case err != nil:
		return err
	case uploadOp == nil:
		return nil // no need to upload, the instance is registered
	}

	// The backend asked us to upload the data to CAS. Do it.
	if err := client.storage.upload(ctx, uploadOp.UploadUrl, instance.DataReader()); err != nil {
		return err
	}
	if err := client.finalizeUpload(ctx, uploadOp.OperationId, timeout); err != nil {
		return err
	}
	logging.Infof(ctx, "cipd: successfully uploaded and verified %s", pin)

	// Try the registration again now that the file is uploaded to CAS. It should
	// succeed.
	switch uploadOp, err := attemptToRegister(); {
	case uploadOp != nil:
		return ErrBadUpload // welp, the upload didn't work for some reason, give up
	default:
		return err
	}
}

// finalizeUpload repeatedly calls FinishUpload RPC until server reports that
// the uploaded file has been verified.
func (client *clientImpl) finalizeUpload(ctx context.Context, opID string, timeout time.Duration) error {
	ctx, cancel := clock.WithTimeout(ctx, timeout)
	defer cancel()

	sleep := time.Second
	for {
		select {
		case <-ctx.Done():
			return ErrFinalizationTimeout
		default:
		}

		op, err := client.cas.FinishUpload(ctx, &api.FinishUploadRequest{
			UploadOperationId: opID,
		})
		switch {
		case err == context.DeadlineExceeded:
			continue // this may be short RPC deadline, try again
		case err != nil:
			return humanErr(err)
		case op.Status == api.UploadStatus_PUBLISHED:
			return nil // verified!
		case op.Status == api.UploadStatus_ERRORED:
			return errors.New(op.ErrorMessage) // fatal verification error
		case op.Status == api.UploadStatus_UPLOADING || op.Status == api.UploadStatus_VERIFYING:
			logging.Infof(ctx, "cipd: uploading - verifying")
			if clock.Sleep(clock.Tag(ctx, "cipd-sleeping"), sleep).Incomplete() {
				return ErrFinalizationTimeout
			}
			if sleep < 10*time.Second {
				sleep += 500 * time.Millisecond
			}
		default:
			return fmt.Errorf("unrecognized upload operation status %s", op.Status)
		}
	}
}

func (client *clientImpl) DescribeInstance(ctx context.Context, pin common.Pin, opts *DescribeInstanceOpts) (*InstanceDescription, error) {
	if err := common.ValidatePin(pin); err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &DescribeInstanceOpts{}
	}

	resp, err := client.repo.DescribeInstance(ctx, &api.DescribeInstanceRequest{
		Package:      pin.PackageName,
		Instance:     common.InstanceIDToObjectRef(pin.InstanceID),
		DescribeRefs: opts.DescribeRefs,
		DescribeTags: opts.DescribeTags,
	}, expectedCodes)
	if err != nil {
		return nil, humanErr(err)
	}

	return apiDescToInfo(resp), nil
}

func (client *clientImpl) SetRefWhenReady(ctx context.Context, ref string, pin common.Pin) error {
	if err := common.ValidatePackageRef(ref); err != nil {
		return err
	}
	if err := common.ValidatePin(pin); err != nil {
		return err
	}
	logging.Infof(ctx, "cipd: setting ref of %q: %q => %q", pin.PackageName, ref, pin.InstanceID)

	err := retryUntilReady(ctx, SetRefTimeout, func(ctx context.Context) error {
		_, err := client.repo.CreateRef(ctx, &api.Ref{
			Name:     ref,
			Package:  pin.PackageName,
			Instance: common.InstanceIDToObjectRef(pin.InstanceID),
		}, expectedCodes)
		return err
	})

	switch err {
	case nil:
		logging.Infof(ctx, "cipd: ref %q is set", ref)
	case ErrProcessingTimeout:
		logging.Errorf(ctx, "cipd: failed to set ref - deadline exceeded")
	default:
		logging.Errorf(ctx, "cipd: failed to set ref - %s", err)
	}
	return err
}

func (client *clientImpl) AttachTagsWhenReady(ctx context.Context, pin common.Pin, tags []string) error {
	if err := common.ValidatePin(pin); err != nil {
		return err
	}
	if len(tags) == 0 {
		return nil
	}

	apiTags := make([]*api.Tag, len(tags))
	for i, t := range tags {
		var err error
		if apiTags[i], err = common.ParseInstanceTag(t); err != nil {
			return err
		}
		logging.Infof(ctx, "cipd: attaching tag %s", t)
	}

	err := retryUntilReady(ctx, TagAttachTimeout, func(ctx context.Context) error {
		_, err := client.repo.AttachTags(ctx, &api.AttachTagsRequest{
			Package:  pin.PackageName,
			Instance: common.InstanceIDToObjectRef(pin.InstanceID),
			Tags:     apiTags,
		}, expectedCodes)
		return err
	})

	switch err {
	case nil:
		logging.Infof(ctx, "cipd: all tags attached")
	case ErrProcessingTimeout:
		logging.Errorf(ctx, "cipd: failed to attach tags - deadline exceeded")
	default:
		logging.Errorf(ctx, "cipd: failed to attach tags - %s", err)
	}
	return err
}

// How long to wait between retries in retryUntilReady.
const retryDelay = 5 * time.Second

// retryUntilReady calls the callback and retries on FailedPrecondition errors,
// which indicate that the instance is not ready yet (still being processed by
// the backend).
func retryUntilReady(ctx context.Context, timeout time.Duration, cb func(context.Context) error) error {
	ctx, cancel := clock.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return ErrProcessingTimeout
		default:
		}

		switch err := cb(ctx); {
		case err == nil:
			return nil
		case err == context.DeadlineExceeded:
			continue // this may be short RPC deadline, try again
		case grpc.Code(err) == codes.FailedPrecondition: // the instance is not ready
			logging.Warningf(ctx, "cipd: %s", humanErr(err))
			if clock.Sleep(clock.Tag(ctx, "cipd-sleeping"), retryDelay).Incomplete() {
				return ErrProcessingTimeout
			}
		default:
			return humanErr(err)
		}
	}
}

func (client *clientImpl) SearchInstances(ctx context.Context, packageName string, tags []string) (common.PinSlice, error) {
	if err := common.ValidatePackageName(packageName); err != nil {
		return nil, err
	}
	if len(tags) == 0 {
		return nil, errors.New("at least one tag is required")
	}

	apiTags := make([]*api.Tag, len(tags))
	for i, t := range tags {
		var err error
		if apiTags[i], err = common.ParseInstanceTag(t); err != nil {
			return nil, err
		}
	}

	resp, err := client.repo.SearchInstances(ctx, &api.SearchInstancesRequest{
		Package:  packageName,
		Tags:     apiTags,
		PageSize: 1000, // TODO(vadimsh): Support pagination on the client.
	}, expectedCodes)
	if err != nil {
		return nil, humanErr(err)
	}

	out := make(common.PinSlice, len(resp.Instances))
	for i, inst := range resp.Instances {
		out[i] = apiInstanceToInfo(inst).Pin
	}
	if resp.NextPageToken != "" {
		logging.Warningf(ctx, "Truncating the result only to first %d instances", len(resp.Instances))
	}
	return out, nil
}

func (client *clientImpl) ListInstances(ctx context.Context, packageName string) (InstanceEnumerator, error) {
	if err := common.ValidatePackageName(packageName); err != nil {
		return nil, err
	}
	return &instanceEnumeratorImpl{
		fetch: func(ctx context.Context, limit int, cursor string) ([]InstanceInfo, string, error) {
			resp, err := client.repo.ListInstances(ctx, &api.ListInstancesRequest{
				Package:   packageName,
				PageSize:  int32(limit),
				PageToken: cursor,
			}, expectedCodes)
			if err != nil {
				return nil, "", humanErr(err)
			}
			instances := make([]InstanceInfo, len(resp.Instances))
			for i, inst := range resp.Instances {
				instances[i] = apiInstanceToInfo(inst)
			}
			return instances, resp.NextPageToken, nil
		},
	}, nil
}

func (client *clientImpl) FetchPackageRefs(ctx context.Context, packageName string) ([]RefInfo, error) {
	if err := common.ValidatePackageName(packageName); err != nil {
		return nil, err
	}

	resp, err := client.repo.ListRefs(ctx, &api.ListRefsRequest{
		Package: packageName,
	}, expectedCodes)
	if err != nil {
		return nil, humanErr(err)
	}

	refs := make([]RefInfo, len(resp.Refs))
	for i, r := range resp.Refs {
		refs[i] = apiRefToInfo(r)
	}
	return refs, nil
}

func (client *clientImpl) FetchInstance(ctx context.Context, pin common.Pin) (local.InstanceFile, error) {
	if err := common.ValidatePin(pin); err != nil {
		return nil, err
	}
	if cache := client.getInstanceCache(ctx); cache != nil {
		return client.fetchInstanceWithCache(ctx, pin, cache)
	}
	return client.fetchInstanceNoCache(ctx, pin)
}

func (client *clientImpl) FetchInstanceTo(ctx context.Context, pin common.Pin, output io.WriteSeeker) error {
	if err := common.ValidatePin(pin); err != nil {
		return err
	}

	// Deal with no-cache situation first, it is simple - just fetch the instance
	// into the 'output'.
	cache := client.getInstanceCache(ctx)
	if cache == nil {
		return client.remoteFetchInstance(ctx, pin, output)
	}

	// If using the cache, always fetch into the cache first, and then copy data
	// from the cache into the output.
	input, err := client.fetchInstanceWithCache(ctx, pin, cache)
	if err != nil {
		return err
	}
	defer func() {
		if err := input.Close(ctx, false); err != nil {
			logging.Warningf(ctx, "cipd: failed to close the package file - %s", err)
		}
	}()

	logging.Infof(ctx, "cipd: copying the instance into the final destination...")
	_, err = io.Copy(output, input)
	return err
}

func (client *clientImpl) fetchInstanceNoCache(ctx context.Context, pin common.Pin) (local.InstanceFile, error) {
	// Use temp file for storing package data. Delete it when the caller is done
	// with it.
	f, err := client.deployer.TempFile(ctx, pin.InstanceID)
	if err != nil {
		return nil, err
	}
	tmp := deleteOnClose{f}

	// Make sure to remove the garbage on errors or panics.
	ok := false
	defer func() {
		if !ok {
			if err := tmp.Close(ctx, false); err != nil {
				logging.Warningf(ctx, "cipd: failed to close the temp file - %s", err)
			}
		}
	}()

	if err := client.remoteFetchInstance(ctx, pin, tmp); err != nil {
		return nil, err
	}

	if _, err := tmp.Seek(0, os.SEEK_SET); err != nil {
		return nil, err
	}
	ok = true
	return tmp, nil
}

func (client *clientImpl) fetchInstanceWithCache(ctx context.Context, pin common.Pin, cache *internal.InstanceCache) (local.InstanceFile, error) {
	attempt := 0
	for {
		attempt++

		// Try to get the instance from cache.
		now := clock.Now(ctx)
		switch file, err := cache.Get(ctx, pin, now); {
		case os.IsNotExist(err):
			// No such package in the cache. This is fine.

		case err != nil:
			// Some unexpected error. Log and carry on, as if it is a cache miss.
			logging.Warningf(ctx, "cipd: could not get %s from cache - %s", pin, err)

		default:
			logging.Infof(ctx, "cipd: instance cache hit for %s", pin)
			return file, nil
		}

		// Download the package into the cache. 'remoteFetchInstance' verifies the
		// hash. When reading from the cache, we can skip the hash check (and we
		// indeed do, see 'cipd: instance cache hit' case above).
		err := cache.Put(ctx, pin, now, func(f *os.File) error {
			return client.remoteFetchInstance(ctx, pin, f)
		})
		if err != nil {
			return nil, err
		}

		// Try to open it now. There's (very) small chance that it has been evicted
		// from the cache already. If this happens, try again. Do it only once.
		//
		// Note that theoretically we could keep open the handle to the file used in
		// 'cache.Put' above, but this file gets renamed at some point, and renaming
		// files with open handles on Windows is moot. So instead we close it,
		// rename the file (this happens inside cache.Put), and reopen it again
		// under the new name.
		file, err := cache.Get(ctx, pin, clock.Now(ctx))
		if err != nil {
			logging.Errorf(ctx, "cipd: %s is unexpectedly missing from cache (%s)", pin, err)
			if attempt == 1 {
				logging.Infof(ctx, "cipd: retrying...")
				continue
			}
			logging.Errorf(ctx, "cipd: giving up")
			return nil, err
		}
		return file, nil
	}
}

// remoteFetchInstance fetches the package file into 'output' and verifies its
// hash along the way. Assumes 'pin' is already validated.
func (client *clientImpl) remoteFetchInstance(ctx context.Context, pin common.Pin, output io.WriteSeeker) (err error) {
	defer func() {
		if err != nil {
			logging.Errorf(ctx, "cipd: failed to fetch %s - %s", pin, err)
		} else {
			logging.Infof(ctx, "cipd: successfully fetched %s", pin)
		}
	}()

	objRef := common.InstanceIDToObjectRef(pin.InstanceID)

	logging.Infof(ctx, "cipd: resolving fetch URL for %s", pin)
	resp, err := client.repo.GetInstanceURL(ctx, &api.GetInstanceURLRequest{
		Package:  pin.PackageName,
		Instance: objRef,
	}, expectedCodes)
	if err != nil {
		return humanErr(err)
	}

	hash := common.MustNewHash(objRef.HashAlgo)
	if err = client.storage.download(ctx, resp.SignedUrl, output, hash); err != nil {
		return
	}

	// Make sure we fetched what we've asked for.
	if digest := common.HexDigest(hash); objRef.HexDigest != digest {
		err = fmt.Errorf("package hash mismatch: expecting %q, got %q", objRef.HexDigest, digest)
	}
	return
}

// fetchAndDo will fetch and open an instance and pass it to the callback.
//
// If the callback fails with an error that indicates a corrupted instance, will
// delete the instance from the cache, refetch it and call the callback again,
// thus the callback should be idempotent.
//
// Any other error from the callback is propagated as is.
func (client *clientImpl) fetchAndDo(ctx context.Context, pin common.Pin, cb func(local.PackageInstance) error) error {
	if err := common.ValidatePin(pin); err != nil {
		return err
	}

	doit := func() (err error) {
		// Fetch the package (verifying its hash) and obtain a pointer to its data.
		instanceFile, err := client.FetchInstance(ctx, pin)
		if err != nil {
			return
		}

		defer func() {
			corrupt := local.IsCorruptionError(err)
			if clErr := instanceFile.Close(ctx, corrupt); clErr != nil && clErr != os.ErrClosed {
				logging.Warningf(ctx, "cipd: failed to close the package file - %s", clErr)
			}
		}()

		// Open the instance. This reads its manifest. 'FetchInstance' has verified
		// the hash already, so skip verification.
		instance, err := local.OpenInstance(ctx, instanceFile, pin.InstanceID, local.SkipHashVerification)
		if err != nil {
			return
		}

		// Opportunistically clean up trashed files.
		defer client.doBatchAwareOp(ctx, batchAwareOpCleanupTrash)

		// Use it. 'defer' will take care of removing the temp file if needed.
		return cb(instance)
	}

	err := doit()
	if err != nil && local.IsCorruptionError(err) {
		logging.WithError(err).Warningf(ctx, "cipd: unpacking failed, retrying.")
		err = doit()
	}
	return err
}

func (client *clientImpl) FetchAndDeployInstance(ctx context.Context, subdir string, pin common.Pin) error {
	if err := common.ValidateSubdir(subdir); err != nil {
		return err
	}
	return client.fetchAndDo(ctx, pin, func(instance local.PackageInstance) error {
		_, err := client.deployer.DeployInstance(ctx, subdir, instance)
		return err
	})
}

func (client *clientImpl) EnsurePackages(ctx context.Context, allPins common.PinSliceBySubdir, paranoia ParanoidMode, dryRun bool) (aMap ActionMap, err error) {
	if err = allPins.Validate(); err != nil {
		return
	}
	if err = paranoia.Validate(); err != nil {
		return
	}

	// needsRepair decided whether we should attempt to repair an already
	// installed package.
	needsRepair := func(string, common.Pin) *RepairPlan { return nil }
	if paranoia != NotParanoid {
		needsRepair = func(subdir string, pin common.Pin) *RepairPlan {
			switch state, err := client.deployer.CheckDeployed(ctx, subdir, pin.PackageName, paranoia, local.WithoutManifest); {
			case err != nil:
				// This error is probably non-recoverable, but we'll try anyway and
				// properly fail later.
				return &RepairPlan{
					NeedsReinstall:  true,
					ReinstallReason: fmt.Sprintf("failed to check the package state - %s", err),
				}
			case !state.Deployed:
				// This should generally not happen. Can probably happen if two clients
				// are messing with same .cipd/* directory concurrently.
				return &RepairPlan{
					NeedsReinstall:  true,
					ReinstallReason: "the package is not deployed at all",
				}
			case state.Pin.InstanceID != pin.InstanceID:
				// Same here.
				return &RepairPlan{
					NeedsReinstall:  true,
					ReinstallReason: fmt.Sprintf("expected to see instance %q, but saw %q", pin.InstanceID, state.Pin.InstanceID),
				}
			case len(state.ToRedeploy) != 0 || len(state.ToRelink) != 0:
				// Have some corrupted files that need to be repaired.
				return &RepairPlan{
					ToRedeploy: state.ToRedeploy,
					ToRelink:   state.ToRelink,
				}
			default:
				return nil // the package needs no repairs
			}
		}
	}

	client.BeginBatch(ctx)
	defer client.EndBatch(ctx)

	// Enumerate existing packages.
	existing, err := client.deployer.FindDeployed(ctx)
	if err != nil {
		return
	}

	// Figure out what needs to be updated and deleted, log it.
	aMap = buildActionPlan(allPins, existing, needsRepair)
	if len(aMap) == 0 {
		logging.Debugf(ctx, "Everything is up-to-date.")
		return
	}
	// TODO(iannucci): ensure that no packages cross root boundaries
	aMap.Log(ctx)

	if dryRun {
		logging.Infof(ctx, "Dry run, not actually doing anything.")
		return
	}

	hasErrors := false

	// Remove all unneeded stuff.
	aMap.LoopOrdered(func(subdir string, actions *Actions) {
		for _, pin := range actions.ToRemove {
			err = client.deployer.RemoveDeployed(ctx, subdir, pin.PackageName)
			if err != nil {
				logging.Errorf(ctx, "Failed to remove %s - %s (subdir %q)", pin.PackageName, err, subdir)
				hasErrors = true
				actions.Errors = append(actions.Errors, ActionError{
					Action: "remove",
					Pin:    pin,
					Error:  JSONError{err},
				})
			}
		}
	})

	// Install all new and updated stuff, repair broken stuff. Install in the
	// order specified by 'pins'. Order matters if multiple packages install same
	// file.
	aMap.LoopOrdered(func(subdir string, actions *Actions) {
		toDeploy := make(map[string]bool, len(actions.ToInstall)+len(actions.ToUpdate))
		toRepair := make(map[string]*RepairPlan, len(actions.ToRepair))
		for _, p := range actions.ToInstall {
			toDeploy[p.PackageName] = true
		}
		for _, pair := range actions.ToUpdate {
			toDeploy[pair.To.PackageName] = true
		}
		for _, broken := range actions.ToRepair {
			if broken.RepairPlan.NeedsReinstall {
				toDeploy[broken.Pin.PackageName] = true
			} else {
				plan := broken.RepairPlan
				toRepair[broken.Pin.PackageName] = &plan
			}
		}
		for _, pin := range allPins[subdir] {
			var action string
			var err error
			if toDeploy[pin.PackageName] {
				action = "install"
				err = client.FetchAndDeployInstance(ctx, subdir, pin)
			} else if plan := toRepair[pin.PackageName]; plan != nil {
				action = "repair"
				err = client.repairDeployed(ctx, subdir, pin, plan)
			}
			if err != nil {
				logging.Errorf(ctx, "Failed to %s %s - %s", action, pin, err)
				hasErrors = true
				actions.Errors = append(actions.Errors, ActionError{
					Action: action,
					Pin:    pin,
					Error:  JSONError{err},
				})
			}
		}
	})

	// Opportunistically cleanup the trash left from previous installs.
	client.doBatchAwareOp(ctx, batchAwareOpCleanupTrash)

	if !hasErrors {
		logging.Infof(ctx, "All changes applied.")
	} else {
		err = ErrEnsurePackagesFailed
	}
	return
}

func (client *clientImpl) repairDeployed(ctx context.Context, subdir string, pin common.Pin, plan *RepairPlan) error {
	// Fetch the package from the backend (or the cache) if some files are really
	// missing. Skip this if we only need to restore symlinks.
	if len(plan.ToRedeploy) != 0 {
		logging.Infof(ctx, "Getting %q to extract %d missing file(s) from it", pin.PackageName, len(plan.ToRedeploy))
		return client.fetchAndDo(ctx, pin, func(instance local.PackageInstance) error {
			return client.deployer.RepairDeployed(ctx, subdir, pin, local.RepairParams{
				Instance:   instance,
				ToRedeploy: plan.ToRedeploy,
				ToRelink:   plan.ToRelink,
			})
		})
	}
	return client.deployer.RepairDeployed(ctx, subdir, pin, local.RepairParams{
		ToRelink: plan.ToRelink,
	})
}

////////////////////////////////////////////////////////////////////////////////
// pRPC error handling.

// gRPC errors that may be returned by api.RepositoryClient that we recognize
// and handle ourselves. They will not be logged by the pRPC library.
var expectedCodes = prpc.ExpectedCode(
	codes.Aborted,
	codes.AlreadyExists,
	codes.FailedPrecondition,
	codes.NotFound,
	codes.PermissionDenied,
)

// humanErr takes gRPC errors and returns a human readable error that can be
// presented in the CLI.
//
// It basically strips scary looking gRPC framing around the error message.
func humanErr(err error) error {
	if err != nil {
		if status, ok := status.FromError(err); ok {
			return errors.New(status.Message())
		}
	}
	return err
}
