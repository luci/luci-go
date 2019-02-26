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
	"context"
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
	"go.chromium.org/luci/cipd/client/cipd/deployer"
	"go.chromium.org/luci/cipd/client/cipd/digests"
	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/platform"
	"go.chromium.org/luci/cipd/client/cipd/reader"
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

var (
	// ClientPackage is a package with the CIPD client. Used during self-update.
	ClientPackage = "infra/tools/cipd/${platform}"
	// UserAgent is HTTP user agent string for CIPD client.
	UserAgent = "cipd 2.2.17"
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
	// its hash matches instance ID in 'pin', and then registers the package in
	// the repository, making it discoverable.
	//
	// 'pin' here should match the package body, otherwise CIPD backend will
	// reject the package. Either get it from builder.BuildInstance (when building
	// a new package) or from reader.CalculatePin (when uploading an existing
	// package file).
	//
	// 'timeout' specifies for how long to wait until the instance hash is
	// verified by the storage backend. If 0, default CASFinalizationTimeout will
	// be used.
	RegisterInstance(ctx context.Context, pin common.Pin, body io.ReadSeeker, timeout time.Duration) error

	// DescribeInstance returns information about a package instance.
	//
	// May also be used as a simple instance presence check, if opts is nil. If
	// the request succeeds, then the instance exists.
	DescribeInstance(ctx context.Context, pin common.Pin, opts *DescribeInstanceOpts) (*InstanceDescription, error)

	// DescribeClient returns information about a CIPD client binary matching the
	// given client package pin.
	DescribeClient(ctx context.Context, pin common.Pin) (*ClientDescription, error)

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
	FetchInstance(ctx context.Context, pin common.Pin) (pkg.Source, error)

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
	// Returns their concrete Pins. If the package doesn't exist at all, returns
	// empty slice and nil error.
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

	// CheckDeployment looks at what is supposed to be installed and compares it
	// to what is really installed.
	//
	// Returns an error if it can't even detect what is supposed to be installed.
	// Inconsistencies are returned through the ActionMap.
	CheckDeployment(ctx context.Context, paranoia ParanoidMode) (ActionMap, error)

	// RepairDeployment attempts to repair a deployment in the site root if it
	// appears to be broken (per given paranoia mode).
	//
	// Returns an action map of what it did.
	RepairDeployment(ctx context.Context, paranoia ParanoidMode) (ActionMap, error)
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

	// Versions is optional database of (pkg, version) => instance ID resolutions.
	//
	// If set, it will be used for all version resolutions done by the client.
	// The client won't be consulting (or updating) the tag cache and won't make
	// 'ResolveVersion' backend RPCs.
	//
	// This is primarily used to implement $ResolvedVersions ensure file feature.
	Versions ensure.VersionsFile

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
		deployer:      deployer.New(opts.Root),
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
// If given 'digests' is not nil, will make sure the hash of the downloaded
// client binary is in 'digests'.
//
// Note that this function make sense only in a context of a default CIPD CLI
// client. Other binaries that link to cipd package should not use it, they'll
// be "updated" to the CIPD client binary.
func MaybeUpdateClient(ctx context.Context, opts ClientOptions, targetVersion, clientExe string, digests *digests.ClientDigestsFile) (common.Pin, error) {
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

	fs := fs.NewFileSystem(opts.Root, filepath.Join(opts.CacheDir, "trash"))
	defer fs.CleanupTrash(ctx)

	pin, err := impl.maybeUpdateClient(ctx, fs, targetVersion, clientExe, digests)
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
	deployer deployer.Deployer

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
	client.deployer.CleanupTrash(ctx)
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
			dir = filepath.Join(client.Root, fs.SiteServiceDir)
		default:
			return
		}
		parsed, err := url.Parse(client.ServiceURL)
		if err != nil {
			panic(err) // the URL has been validated in NewClient already
		}
		client.tagCache = internal.NewTagCache(fs.NewFileSystem(dir, ""), parsed.Host)
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
		client.instanceCache = internal.NewInstanceCache(fs.NewFileSystem(path, ""))
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

	// Is it instance ID already? Then it is already resolved.
	if common.ValidateInstanceID(version, common.AnyHash) == nil {
		return common.Pin{PackageName: packageName, InstanceID: version}, nil
	}
	if err := common.ValidateInstanceVersion(version); err != nil {
		return common.Pin{}, err
	}

	// Use the preresolved version if configured to do so. Do NOT fallback to
	// the backend calls. A missing version is an error.
	if client.Versions != nil {
		return client.Versions.ResolveVersion(packageName, version)
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
func (client *clientImpl) ensureClientVersionInfo(ctx context.Context, fs fs.FileSystem, pin common.Pin, clientExe string) {
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
func (client *clientImpl) maybeUpdateClient(ctx context.Context, fs fs.FileSystem,
	targetVersion, clientExe string, digests *digests.ClientDigestsFile) (common.Pin, error) {

	// currentHashMatches calculates the existing client binary hash and compares
	// it to 'obj'.
	currentHashMatches := func(obj *api.ObjectRef) (yep bool, err error) {
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
		return common.HexDigest(hash) == obj.HexDigest, nil
	}

	client.BeginBatch(ctx)
	defer client.EndBatch(ctx)

	// Resolve the client version to a pin, to be able to later grab URL to the
	// binary by querying info for that pin.
	var pin common.Pin
	clientPackage, err := template.DefaultExpander().Expand(ClientPackage)
	if err != nil {
		return common.Pin{}, err // shouldn't be happening in reality
	}
	if pin, err = client.ResolveVersion(ctx, clientPackage, targetVersion); err != nil {
		return common.Pin{}, err
	}

	// The name of the client binary inside the client CIPD package. Acts only as
	// a key inside the extracted refs cache, nothing more, so can technically be
	// arbitrary.
	clientFileName := "cipd"
	if platform.CurrentOS() == "windows" {
		clientFileName = "cipd.exe"
	}

	// rememberClientRef populates the extracted refs cache.
	rememberClientRef := func(pin common.Pin, ref *api.ObjectRef) {
		if cache := client.getTagCache(); cache != nil {
			cache.AddExtractedObjectRef(ctx, pin, clientFileName, ref)
			client.doBatchAwareOp(ctx, batchAwareOpSaveTagCache)
		}
	}

	// Look up the hash corresponding to the pin in the extracted refs cache. See
	// rememberClientRef calls below for where it is stored initially. A cache
	// miss is fine, we'll reach to the backend to get the hash. A warm cache
	// allows skipping RPCs to the backend on a "happy path", when the client is
	// already up-to-date.
	var clientRef *api.ObjectRef
	if cache := client.getTagCache(); cache != nil {
		if clientRef, err = cache.ResolveExtractedObjectRef(ctx, pin, clientFileName); err != nil {
			return common.Pin{}, err
		}
	}

	// If not using the tags cache or it is cold, ask the backend for an expected
	// client ref. Note that we do it even if we have 'digests' file available,
	// to handle the case when 'digests' file is stale (which can happen when
	// updating the client version file).
	var info *ClientDescription
	if clientRef == nil {
		if info, err = client.DescribeClient(ctx, pin); err != nil {
			return common.Pin{}, err
		}
		clientRef = info.Digest
	}

	// If using pinned client digests, make sure the hash reported by the backend
	// is mentioned there. In most cases a mismatch means the pinned digests file
	// is just stale. The mismatch can also happen if the backend is compromised
	// or the client package was forcefully replaced (this should never really
	// happen...).
	if digests != nil {
		plat := platform.CurrentPlatform()
		switch pinnedRef := digests.ClientRef(plat); {
		case pinnedRef == nil:
			return common.Pin{}, fmt.Errorf("there's no supported hash for %q in CIPD *.digests file", plat)
		case !digests.Contains(plat, clientRef):
			return common.Pin{}, fmt.Errorf(
				"the CIPD client hash reported by the backend (%s) is not in *.digests file, "+
					"if you changed CIPD client version recently most likely the *.digests "+
					"file is just stale and needs to be regenerated via 'cipd selfupdate-roll ...'",
				clientRef.HexDigest)
		default:
			clientRef = pinnedRef // pick the best supported hash algo from *.digests
		}
	}

	// Is the client binary already up-to-date (has the expected hash)?
	switch yep, err := currentHashMatches(clientRef); {
	case err != nil:
		return common.Pin{}, err // can't read clientExe
	case yep:
		// If we had to fetch the expected hash, store it in the cache to avoid
		// fetching it again. Don't do it if we read it from the cache initially (to
		// skip unnecessary cache write).
		if info != nil {
			rememberClientRef(pin, clientRef)
		}
		return pin, nil
	}

	if targetVersion == pin.InstanceID {
		logging.Infof(ctx, "cipd: updating client to %s", pin)
	} else {
		logging.Infof(ctx, "cipd: updating client to %s (%s)", pin, targetVersion)
	}

	// Grab the signed URL of the client binary if we haven't done so already.
	if info == nil {
		if info, err = client.DescribeClient(ctx, pin); err != nil {
			return common.Pin{}, err
		}
	}

	// Here we know for sure that the current binary has wrong hash (most likely
	// it is outdated). Fetch the new binary, verifying its hash matches the one
	// we expect.
	err = client.installClient(
		ctx, fs,
		common.MustNewHash(clientRef.HashAlgo),
		info.SignedUrl,
		clientExe,
		clientRef.HexDigest)
	if err != nil {
		// Either a download error or hash mismatch.
		return common.Pin{}, errors.Annotate(err, "when updating the CIPD client to %q", targetVersion).Err()
	}

	// The new fetched binary is valid.
	rememberClientRef(pin, clientRef)
	return pin, nil
}

func (client *clientImpl) RegisterInstance(ctx context.Context, pin common.Pin, body io.ReadSeeker, timeout time.Duration) error {
	if timeout == 0 {
		timeout = CASFinalizationTimeout
	}

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
	if err := client.storage.upload(ctx, uploadOp.UploadUrl, body); err != nil {
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
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
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

func (client *clientImpl) DescribeClient(ctx context.Context, pin common.Pin) (*ClientDescription, error) {
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
		return nil, err
	}

	resp, err := client.repo.DescribeClient(ctx, &api.DescribeClientRequest{
		Package:  pin.PackageName,
		Instance: common.InstanceIDToObjectRef(pin.InstanceID),
	}, expectedCodes)
	if err != nil {
		return nil, humanErr(err)
	}

	return apiClientDescToInfo(resp), nil
}

func (client *clientImpl) SetRefWhenReady(ctx context.Context, ref string, pin common.Pin) error {
	if err := common.ValidatePackageRef(ref); err != nil {
		return err
	}
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
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
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
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
	switch {
	case status.Code(err) == codes.NotFound: // no such package => no instances
		return nil, nil
	case err != nil:
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

func (client *clientImpl) FetchInstance(ctx context.Context, pin common.Pin) (pkg.Source, error) {
	if err := common.ValidatePin(pin, common.KnownHash); err != nil {
		return nil, err
	}
	if cache := client.getInstanceCache(ctx); cache != nil {
		return client.fetchInstanceWithCache(ctx, pin, cache)
	}
	return client.fetchInstanceNoCache(ctx, pin)
}

func (client *clientImpl) FetchInstanceTo(ctx context.Context, pin common.Pin, output io.WriteSeeker) error {
	if err := common.ValidatePin(pin, common.KnownHash); err != nil {
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

func (client *clientImpl) fetchInstanceNoCache(ctx context.Context, pin common.Pin) (pkg.Source, error) {
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

func (client *clientImpl) fetchInstanceWithCache(ctx context.Context, pin common.Pin, cache *internal.InstanceCache) (pkg.Source, error) {
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
	startTS := clock.Now(ctx)
	defer func() {
		if err != nil {
			logging.Errorf(ctx, "cipd: failed to fetch %s - %s", pin, err)
		} else {
			logging.Infof(ctx, "cipd: successfully fetched %s in %.1fs", pin, clock.Now(ctx).Sub(startTS).Seconds())
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
func (client *clientImpl) fetchAndDo(ctx context.Context, pin common.Pin, cb func(pkg.Instance) error) error {
	if err := common.ValidatePin(pin, common.KnownHash); err != nil {
		return err
	}

	doit := func() (err error) {
		// Fetch the package (verifying its hash) and obtain a pointer to its data.
		instanceFile, err := client.FetchInstance(ctx, pin)
		if err != nil {
			return
		}

		// Notify the underlying object if 'err' is a corruption error.
		type corruptable interface {
			Close(ctx context.Context, corrupt bool) error
		}
		closeMaybeCorrupted := func(f corruptable) {
			corrupt := reader.IsCorruptionError(err)
			if clErr := f.Close(ctx, corrupt); clErr != nil && clErr != os.ErrClosed {
				logging.Warningf(ctx, "cipd: failed to close the package file - %s", clErr)
			}
		}

		// Open the instance. This reads its manifest. 'FetchInstance' has verified
		// the hash already, so skip the verification.
		instance, err := reader.OpenInstance(ctx, instanceFile, reader.OpenInstanceOpts{
			VerificationMode: reader.SkipHashVerification,
			InstanceID:       pin.InstanceID,
		})
		if err != nil {
			closeMaybeCorrupted(instanceFile)
			return
		}

		defer client.doBatchAwareOp(ctx, batchAwareOpCleanupTrash)
		defer closeMaybeCorrupted(instance)

		// Use it. 'defer' will take care of removing the temp file if needed.
		return cb(instance)
	}

	err := doit()
	if err != nil && reader.IsCorruptionError(err) {
		logging.WithError(err).Warningf(ctx, "cipd: unpacking failed, retrying.")
		err = doit()
	}
	return err
}

func (client *clientImpl) FetchAndDeployInstance(ctx context.Context, subdir string, pin common.Pin) error {
	if err := common.ValidateSubdir(subdir); err != nil {
		return err
	}
	return client.fetchAndDo(ctx, pin, func(instance pkg.Instance) error {
		_, err := client.deployer.DeployInstance(ctx, subdir, instance)
		return err
	})
}

func (client *clientImpl) EnsurePackages(ctx context.Context, allPins common.PinSliceBySubdir, paranoia ParanoidMode, dryRun bool) (ActionMap, error) {
	return client.ensurePackagesImpl(ctx, allPins, paranoia, dryRun, false)
}

func (client *clientImpl) ensurePackagesImpl(ctx context.Context, allPins common.PinSliceBySubdir, paranoia ParanoidMode, dryRun, silent bool) (aMap ActionMap, err error) {
	if err = allPins.Validate(common.AnyHash); err != nil {
		return
	}
	if err = paranoia.Validate(); err != nil {
		return
	}

	client.BeginBatch(ctx)
	defer client.EndBatch(ctx)

	// Enumerate existing packages.
	existing, err := client.deployer.FindDeployed(ctx)
	if err != nil {
		return
	}

	// Figure out what needs to be updated and deleted, log it.
	aMap = buildActionPlan(allPins, existing, client.makeRepairChecker(ctx, paranoia))
	if len(aMap) == 0 {
		if !silent {
			logging.Debugf(ctx, "Everything is up-to-date.")
		}
		return
	}

	// TODO(iannucci): ensure that no packages cross root boundaries
	if !silent {
		aMap.Log(ctx, false)
	}
	if dryRun {
		if !silent {
			logging.Infof(ctx, "Dry run, not actually doing anything.")
		}
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

func (client *clientImpl) CheckDeployment(ctx context.Context, paranoia ParanoidMode) (ActionMap, error) {
	// This is essentially a dry run of EnsurePackages(already installed pkgs),
	// but with some paranoia mode, so it detects breakages.
	existing, err := client.deployer.FindDeployed(ctx)
	if err != nil {
		return nil, err
	}
	return client.ensurePackagesImpl(ctx, existing, paranoia, true, true)
}

func (client *clientImpl) RepairDeployment(ctx context.Context, paranoia ParanoidMode) (ActionMap, error) {
	// And this is a real run of EnsurePackages(already installed pkgs), so it
	// can do repairs, if necessary.
	existing, err := client.deployer.FindDeployed(ctx)
	if err != nil {
		return nil, err
	}
	return client.EnsurePackages(ctx, existing, paranoia, false)
}

// makeRepairChecker returns a function that decided whether we should attempt
// to repair an already installed package.
//
// The implementation depends on selected paranoia mode.
func (client *clientImpl) makeRepairChecker(ctx context.Context, paranoia ParanoidMode) repairCB {
	if paranoia == NotParanoid {
		return func(string, common.Pin) *RepairPlan { return nil }
	}

	return func(subdir string, pin common.Pin) *RepairPlan {
		switch state, err := client.deployer.CheckDeployed(ctx, subdir, pin.PackageName, paranoia, WithoutManifest); {
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

func (client *clientImpl) repairDeployed(ctx context.Context, subdir string, pin common.Pin, plan *RepairPlan) error {
	// Fetch the package from the backend (or the cache) if some files are really
	// missing. Skip this if we only need to restore symlinks.
	if len(plan.ToRedeploy) != 0 {
		logging.Infof(ctx, "Getting %q to extract %d missing file(s) from it", pin.PackageName, len(plan.ToRedeploy))
		return client.fetchAndDo(ctx, pin, func(instance pkg.Instance) error {
			return client.deployer.RepairDeployed(ctx, subdir, pin, deployer.RepairParams{
				Instance:   instance,
				ToRedeploy: plan.ToRedeploy,
				ToRelink:   plan.ToRelink,
			})
		})
	}
	return client.deployer.RepairDeployed(ctx, subdir, pin, deployer.RepairParams{
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
