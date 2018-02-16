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
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/cipd/client/cipd/common"
	"go.chromium.org/luci/cipd/client/cipd/internal"
	"go.chromium.org/luci/cipd/client/cipd/local"
	"go.chromium.org/luci/cipd/client/cipd/platform"
	"go.chromium.org/luci/cipd/version"
)

// PackageACLChangeAction defines a flavor of PackageACLChange.
type PackageACLChangeAction string

const (
	// GrantRole is used in PackageACLChange to request a role to be granted.
	GrantRole PackageACLChangeAction = "GRANT"
	// RevokeRole is used in PackageACLChange to request a role to be revoked.
	RevokeRole PackageACLChangeAction = "REVOKE"

	// CASFinalizationTimeout is how long to wait for CAS service to finalize the upload.
	CASFinalizationTimeout = 5 * time.Minute
	// SetRefTimeout is how long to wait for an instance to be processed when setting a ref.
	SetRefTimeout = 3 * time.Minute
	// TagAttachTimeout is how long to wait for an instance to be processed when attaching tags.
	TagAttachTimeout = 3 * time.Minute
)

// Environment variable definitions
const (
	EnvCacheDir            = "CIPD_CACHE_DIR"
	EnvHTTPUserAgentPrefix = "CIPD_HTTP_USER_AGENT_PREFIX"
)

var (
	// ErrFinalizationTimeout is returned if CAS service can not finalize upload fast enough.
	ErrFinalizationTimeout = errors.New("timeout while waiting for CAS service to finalize the upload", transient.Tag)
	// ErrBadUpload is returned when a package file is uploaded, but servers asks us to upload it again.
	ErrBadUpload = errors.New("package file is uploaded, but servers asks us to upload it again", transient.Tag)
	// ErrBadUploadSession is returned by UploadToCAS if provided UploadSession is not valid.
	ErrBadUploadSession = errors.New("uploadURL must be set if UploadSessionID is used")
	// ErrUploadSessionDied is returned by UploadToCAS if upload session suddenly disappeared.
	ErrUploadSessionDied = errors.New("upload session is unexpectedly missing", transient.Tag)
	// ErrNoUploadSessionID is returned by UploadToCAS if server didn't provide upload session ID.
	ErrNoUploadSessionID = errors.New("server didn't provide upload session ID")
	// ErrSetRefTimeout is returned when service refuses to move a ref for a long time.
	ErrSetRefTimeout = errors.New("timeout while moving a ref", transient.Tag)
	// ErrAttachTagsTimeout is returned when service refuses to accept tags for a long time.
	ErrAttachTagsTimeout = errors.New("timeout while attaching tags", transient.Tag)
	// ErrDownloadError is returned by FetchInstance on download errors.
	ErrDownloadError = errors.New("failed to download the package file after multiple attempts", transient.Tag)
	// ErrUploadError is returned by RegisterInstance and UploadToCAS on upload errors.
	ErrUploadError = errors.New("failed to upload the package file after multiple attempts", transient.Tag)
	// ErrAccessDenined is returned by calls talking to backend on 401 or 403 HTTP errors.
	ErrAccessDenined = errors.New("access denied (not authenticated or not enough permissions)")
	// ErrBackendInaccessible is returned by calls talking to backed if it doesn't response.
	ErrBackendInaccessible = errors.New("request to the backend failed after multiple attempts", transient.Tag)
	// ErrEnsurePackagesFailed is returned by EnsurePackages if something is not right.
	ErrEnsurePackagesFailed = errors.New("failed to update packages, see the log")
	// ErrPackageNotFound is returned by DeletePackage if the package doesn't exist.
	ErrPackageNotFound = errors.New("no such package")
)

var (
	// UserAgent is HTTP user agent string for CIPD client.
	UserAgent = "cipd 1.7.4"
)

func init() {
	ver, err := version.GetStartupVersion()
	if err != nil || ver.InstanceID == "" {
		return
	}
	UserAgent += fmt.Sprintf(" (%s@%s)", ver.PackageName, ver.InstanceID)
}

// UnixTime is time.Time that serializes to unix timestamp in JSON (represented
// as a number of seconds since January 1, 1970 UTC).
type UnixTime time.Time

// String is needed to be able to print UnixTime.
func (t UnixTime) String() string {
	return time.Time(t).String()
}

// Before is used to compare UnixTime objects.
func (t UnixTime) Before(t2 UnixTime) bool {
	return time.Time(t).Before(time.Time(t2))
}

// IsZero reports whether t represents the zero time instant.
func (t UnixTime) IsZero() bool {
	return time.Time(t).IsZero()
}

// MarshalJSON is used by JSON encoder.
func (t UnixTime) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("0"), nil
	}
	return []byte(fmt.Sprintf("%d", time.Time(t).Unix())), nil
}

// JSONError is wrapper around Error that serializes it as string.
type JSONError struct {
	error
}

// MarshalJSON is used by JSON encoder.
func (e JSONError) MarshalJSON() ([]byte, error) {
	return []byte(e.Error()), nil
}

// PackageACL is per package path per role access control list that is a part of
// larger overall ACL: ACL for package "a/b/c" is a union of PackageACLs for "a"
// "a/b" and "a/b/c".
type PackageACL struct {
	// PackagePath is a package subpath this ACL is defined for.
	PackagePath string `json:"package_path"`
	// Role is a role that listed users have, e.g. 'READER', 'WRITER', ...
	Role string `json:"role"`
	// Principals list users and groups granted the role.
	Principals []string `json:"principals"`
	// ModifiedBy specifies who modified the list the last time.
	ModifiedBy string `json:"modified_by"`
	// ModifiedTs is a timestamp when the list was modified the last time.
	ModifiedTs UnixTime `json:"modified_ts"`
}

// PackageACLChange is a mutation to some package ACL.
type PackageACLChange struct {
	// Action defines what action to perform: GrantRole or RevokeRole.
	Action PackageACLChangeAction
	// Role to grant or revoke to a user or group.
	Role string
	// Principal is a user or a group to grant or revoke a role for.
	Principal string
}

// UploadSession describes open CAS upload session.
type UploadSession struct {
	// ID identifies upload session in the backend.
	ID string
	// URL is where to upload the data to.
	URL string
}

// InstanceInfo is information about single package instance.
type InstanceInfo struct {
	// Pin identifies package instance.
	Pin common.Pin `json:"pin"`
	// RegisteredBy is identity of whoever uploaded this instance.
	RegisteredBy string `json:"registered_by"`
	// RegisteredTs is when the instance was registered.
	RegisteredTs UnixTime `json:"registered_ts"`
}

// TagInfo is returned by FetchInstanceTags.
type TagInfo struct {
	// Tag is actual tag name ("key:value" pair).
	Tag string `json:"tag"`
	// RegisteredBy is identity of whoever attached this tag.
	RegisteredBy string `json:"registered_by"`
	// RegisteredTs is when the tag was registered.
	RegisteredTs UnixTime `json:"registered_ts"`
}

// RefInfo is returned by FetchInstanceRefs and FetchPackageRefs.
type RefInfo struct {
	// Ref is the ref name.
	Ref string `json:"ref"`
	// InstanceID is ID of a package instance the ref points to.
	InstanceID string `json:"instance_id"`
	// ModifiedBy is identity of whoever modified this ref last time.
	ModifiedBy string `json:"modified_by"`
	// ModifiedTs is when the ref was modified last time.
	ModifiedTs UnixTime `json:"modified_ts"`
}

// Counter is returned by ReadCounter.
type Counter struct {
	// Name is the counter's name.
	Name string `json:"name"`
	// Value is the counter's value.
	Value int64 `json:"value"`
	// CreatedTS is the first time the counter was written.
	CreatedTS UnixTime `json:"created_ts"`
	// UpdatedTS is the most recent time the counter was written.
	UpdatedTS UnixTime `json:"updated_ts"`
}

// ActionMap is a map of subdir to the Actions which will occur within it.
type ActionMap map[string]*Actions

// LoopOrdered loops over the ActionMap in sorted order (by subdir).
func (am ActionMap) LoopOrdered(cb func(subdir string, actions *Actions)) {
	subdirs := make(sort.StringSlice, 0, len(am))
	for subdir := range am {
		subdirs = append(subdirs, subdir)
	}
	subdirs.Sort()
	for _, subdir := range subdirs {
		cb(subdir, am[subdir])
	}
}

// Log prints the pending action to the logger installed in ctx.
func (am ActionMap) Log(ctx context.Context) {
	keys := make([]string, 0, len(am))
	for key := range am {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, subdir := range keys {
		actions := am[subdir]

		if subdir == "" {
			logging.Infof(ctx, "In root:")
		} else {
			logging.Infof(ctx, "In subdir %q:", subdir)
		}

		if len(actions.ToInstall) != 0 {
			logging.Infof(ctx, "  to install:")
			for _, pin := range actions.ToInstall {
				logging.Infof(ctx, "    %s", pin)
			}
		}
		if len(actions.ToUpdate) != 0 {
			logging.Infof(ctx, "  to update:")
			for _, pair := range actions.ToUpdate {
				logging.Infof(ctx, "    %s (%s -> %s)",
					pair.From.PackageName, pair.From.InstanceID, pair.To.InstanceID)
			}
		}
		if len(actions.ToRemove) != 0 {
			logging.Infof(ctx, "  to remove:")
			for _, pin := range actions.ToRemove {
				logging.Infof(ctx, "    %s", pin)
			}
		}
	}
}

// Actions is returned by EnsurePackages.
//
// It lists pins that were attempted to be installed, updated or removed, as
// well as all errors.
type Actions struct {
	ToInstall common.PinSlice `json:"to_install,omitempty"` // pins to be installed
	ToUpdate  []UpdatedPin    `json:"to_update,omitempty"`  // pins to be replaced
	ToRemove  common.PinSlice `json:"to_remove,omitempty"`  // pins to be removed
	Errors    []ActionError   `json:"errors,omitempty"`     // all individual errors
}

// Empty is true if there are no actions specified.
func (a *Actions) Empty() bool {
	return len(a.ToInstall) == 0 && len(a.ToUpdate) == 0 && len(a.ToRemove) == 0
}

// UpdatedPin specifies a pair of pins: old and new version of a package.
type UpdatedPin struct {
	From common.Pin `json:"from"`
	To   common.Pin `json:"to"`
}

// ActionError holds an error that happened when installing or removing the pin.
type ActionError struct {
	Action string     `json:"action"`
	Pin    common.Pin `json:"pin"`
	Error  JSONError  `json:"error,omitempty"`
}

// ReadSeekCloser is the interface that groups Reader, Seeker and Closer.
type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
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
	// Together they define the access control list for the given package subpath.
	FetchACL(ctx context.Context, packagePath string) ([]PackageACL, error)

	// ModifyACL applies a set of PackageACLChanges to a package path.
	ModifyACL(ctx context.Context, packagePath string, changes []PackageACLChange) error

	// UploadToCAS uploads package data blob to Content Addressed Store.
	//
	// Does nothing if it is already there. The data is addressed by SHA1 hash
	// (also known as package's InstanceID). It can be used as a standalone
	// function (if 'session' is nil) or as a part of more high level upload
	// process (in that case upload session can be opened elsewhere and its
	// properties passed here via 'session' argument).
	//
	// Returns nil on successful upload.
	UploadToCAS(ctx context.Context, sha1 string, data io.ReadSeeker, session *UploadSession, timeout time.Duration) error

	// ResolveVersion converts an instance ID, a tag or a ref into a concrete Pin.
	ResolveVersion(ctx context.Context, packageName, version string) (common.Pin, error)

	// MaybeUpdateClient will update `destination` to `targetVersion` if
	// `currentHash` doesn't match version's executable hash.
	//
	// This update is done from the "infra/tools/cipd/${os}-${arch}" package.
	MaybeUpdateClient(ctx context.Context, fs local.FileSystem, targetVersion, currentHash, destination string) error

	// RegisterInstance makes the package instance available for clients.
	//
	// It uploads the instance to the storage and registers it in the package
	// repository.
	RegisterInstance(ctx context.Context, instance local.PackageInstance, timeout time.Duration) error

	// DeletePackage removes the package (all its instances) from the backend.
	//
	// It will delete all package instances, all tags and refs. There's no undo.
	DeletePackage(ctx context.Context, packageName string) error

	// SetRefWhenReady moves a ref to point to a package instance.
	SetRefWhenReady(ctx context.Context, ref string, pin common.Pin) error

	// AttachTagsWhenReady attaches tags to an instance.
	AttachTagsWhenReady(ctx context.Context, pin common.Pin, tags []string) error

	// FetchInstanceInfo returns general information about the instance.
	FetchInstanceInfo(ctx context.Context, pin common.Pin) (InstanceInfo, error)

	// FetchInstanceTags returns information about tags attached to the instance.
	//
	// The returned list is sorted by tag key and creation timestamp (newest
	// first). If 'tags' is empty, fetches all attached tags, otherwise only
	// ones specified.
	FetchInstanceTags(ctx context.Context, pin common.Pin, tags []string) ([]TagInfo, error)

	// FetchInstanceRefs returns information about refs pointing to the instance.
	//
	// The returned list is sorted by modification timestamp (newest first). If
	// 'refs' is empty, fetches all refs, otherwise only ones specified.
	FetchInstanceRefs(ctx context.Context, pin common.Pin, refs []string) ([]RefInfo, error)

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

	// ListPackages returns a list of strings of package names.
	ListPackages(ctx context.Context, path string, recursive, showHidden bool) ([]string, error)

	// SearchInstances finds all instances with given tag and optionally name.
	//
	// Returns their concrete Pins.
	SearchInstances(ctx context.Context, tag, packageName string) (common.PinSlice, error)

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
	// If dryRun is true, will just check for changes and return them in Actions
	// struct, but won't actually perform them.
	//
	// If the update was only partially applied, returns both Actions and error.
	EnsurePackages(ctx context.Context, pkgs common.PinSliceBySubdir, dryRun bool) (ActionMap, error)

	// IncrementCounter adds delta to the counter's value and updates its last
	// updated timestamp.
	//
	// delta must be 0 or 1.
	IncrementCounter(ctx context.Context, pin common.Pin, counterName string, delta int) error

	// ReadCounter returns the current value of the counter.
	ReadCounter(ctx context.Context, pin common.Pin, counterName string) (Counter, error)
}

// InstanceEnumerator produces a list of instances, fetching them in batches.
type InstanceEnumerator interface {
	// Next returns next up to 'limit' instances or 0 if there's no more.
	Next(ctx context.Context, limit int) ([]InstanceInfo, error)
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

	return &clientImpl{
		ClientOptions: opts,
		remote: &remoteImpl{
			serviceURL: opts.ServiceURL,
			userAgent:  opts.UserAgent,
			client:     opts.AuthenticatedClient,
		},
		storage: &storageImpl{
			chunkSize: uploadChunkSize,
			userAgent: opts.UserAgent,
			client:    opts.AnonymousClient,
		},
		deployer: local.NewDeployer(opts.Root),
	}, nil
}

type clientImpl struct {
	ClientOptions

	// batchLock protects guts of by BeginBatch/EndBatch implementation.
	batchLock    sync.Mutex
	batchNesting int
	batchPending map[batchAwareOp]struct{}

	// remote knows how to call backend REST API.
	remote remote

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

func (client *clientImpl) FetchACL(ctx context.Context, packagePath string) ([]PackageACL, error) {
	return client.remote.fetchACL(ctx, packagePath)
}

func (client *clientImpl) ModifyACL(ctx context.Context, packagePath string, changes []PackageACLChange) error {
	return client.remote.modifyACL(ctx, packagePath, changes)
}

func (client *clientImpl) ListPackages(ctx context.Context, path string, recursive, showHidden bool) ([]string, error) {
	pkgs, dirs, err := client.remote.listPackages(ctx, path, recursive, showHidden)
	if err != nil {
		return nil, err
	}

	// Add trailing slash to directories.
	for k, d := range dirs {
		dirs[k] = d + "/"
	}
	// Merge and sort packages and directories.
	allPkgs := append(pkgs, dirs...)
	sort.Strings(allPkgs)
	return allPkgs, nil
}

func (client *clientImpl) UploadToCAS(ctx context.Context, sha1 string, data io.ReadSeeker, session *UploadSession, timeout time.Duration) error {
	// Open new upload session if an existing is not provided.
	var err error
	if session == nil {
		logging.Infof(ctx, "cipd: uploading %s: initiating", sha1)
		session, err = client.remote.initiateUpload(ctx, sha1)
		if err != nil {
			logging.Warningf(ctx, "cipd: can't upload %s - %s", sha1, err)
			return err
		}
		if session == nil {
			logging.Infof(ctx, "cipd: %s is already uploaded", sha1)
			return nil
		}
	} else {
		if session.ID == "" || session.URL == "" {
			return ErrBadUploadSession
		}
	}

	// Upload the file to CAS storage.
	err = client.storage.upload(ctx, session.URL, data)
	if err != nil {
		return err
	}

	// Finalize the upload, wait until server verifies and publishes the file.
	if timeout == 0 {
		timeout = CASFinalizationTimeout
	}
	started := clock.Now(ctx)
	delay := time.Second
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		published, err := client.remote.finalizeUpload(ctx, session.ID)
		if err != nil {
			logging.Warningf(ctx, "cipd: upload of %s failed: %s", sha1, err)
			return err
		}
		if published {
			logging.Infof(ctx, "cipd: successfully uploaded %s", sha1)
			return nil
		}
		if clock.Now(ctx).Sub(started) > timeout {
			logging.Warningf(ctx, "cipd: upload of %s failed: timeout", sha1)
			return ErrFinalizationTimeout
		}
		logging.Infof(ctx, "cipd: uploading - verifying")
		clock.Sleep(ctx, delay)
		if delay < 4*time.Second {
			delay += 500 * time.Millisecond
		}
	}
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
	// Use local cache when resolving tags to avoid round trips to backend when
	// calling same 'cipd ensure' command again and again.
	var cache *internal.TagCache
	if common.ValidateInstanceTag(version) == nil {
		cache = client.getTagCache()
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
	pin, err := client.remote.resolveVersion(ctx, packageName, version)
	if err != nil {
		return pin, err
	}
	if cache != nil {
		if err := cache.AddTag(ctx, pin, version); err != nil {
			logging.Warningf(ctx, "cipd: could not add tag to the cache")
		}
		client.doBatchAwareOp(ctx, batchAwareOpSaveTagCache)
	}
	return pin, nil
}

const clientPackageBase = "infra/tools/cipd"

var clientPackage = ""
var clientFileName = ""

func init() {
	clientFileName = "cipd"
	if platform.CurrentOS() == "windows" {
		clientFileName = "cipd.exe"
	}

	clientPackage = fmt.Sprintf("%s/%s-%s", clientPackageBase,
		platform.CurrentOS(), platform.CurrentArchitecture())
}

func (client *clientImpl) ensureClientVersionInfo(ctx context.Context, fs local.FileSystem, pin common.Pin, exePath string) {
	verFile := version.GetVersionFile(exePath)

	expect, err := json.Marshal(version.Info{
		PackageName: pin.PackageName,
		InstanceID:  pin.InstanceID,
	})
	if err != nil {
		// should never occur; only error could be if version.Info is not JSON
		// serializable.
		logging.WithError(err).Errorf(ctx, "Unable to generate version file content")
		return
	}

	if f, err := os.Open(verFile); err == nil {
		data, err := ioutil.ReadAll(f)
		f.Close()
		if err == nil && bytes.Equal(expect, data) {
			// up to date
			return
		}
	}
	// there was an error reading the existing version file, or its content does
	// not match. Proceed with EnsureFile.

	err = fs.EnsureFile(ctx, verFile, func(of *os.File) error {
		_, err := of.Write(expect)
		return err
	})
	if err != nil {
		logging.WithError(err).Warningf(ctx, "Unable to update version info %q", verFile)
	}
}

func (client *clientImpl) MaybeUpdateClient(ctx context.Context, fs local.FileSystem, targetVersion, currentHash, destination string) error {
	pin, err := client.maybeUpdateClient(ctx, fs, targetVersion, currentHash, destination)
	if err == nil {
		client.ensureClientVersionInfo(ctx, fs, pin, destination)
	}
	return err
}

func (client *clientImpl) maybeUpdateClient(ctx context.Context, fs local.FileSystem, targetVersion, currentHash, destination string) (pin common.Pin, err error) {
	if err = common.ValidateFileHash(currentHash); err != nil {
		return
	}

	client.BeginBatch(ctx)
	defer client.EndBatch(ctx)

	if pin, err = client.ResolveVersion(ctx, clientPackage, targetVersion); err != nil {
		return
	}

	cache := client.getTagCache()
	exeHash := ""
	if cache != nil {
		if exeHash, err = cache.ResolveFile(ctx, pin, clientFileName); err != nil {
			return
		}
	}
	if exeHash == currentHash {
		// already up-to-date. Make sure version file is up to date.
		return
	}

	if targetVersion == pin.InstanceID {
		logging.Infof(ctx, "cipd: updating client to %s", pin)
	} else {
		logging.Infof(ctx, "cipd: updating client to %s (%s)", pin, targetVersion)
	}

	info, err := client.remote.fetchClientBinaryInfo(ctx, pin)
	if err != nil {
		return
	}
	if cache != nil {
		if err = cache.AddFile(ctx, pin, clientFileName, info.clientBinary.SHA1); err != nil {
			return
		}
	}
	client.doBatchAwareOp(ctx, batchAwareOpSaveTagCache)
	if info.clientBinary.SHA1 == currentHash {
		// already up-to-date, but the cache didn't know that. Make sure version
		// file is update.
		return
	}

	err = client.installClient(ctx, fs, sha1.New(), info.clientBinary.FetchURL, destination, info.clientBinary.SHA1)
	if err != nil {
		return
	}

	return
}

func (client *clientImpl) RegisterInstance(ctx context.Context, instance local.PackageInstance, timeout time.Duration) error {
	// Attempt to register.
	logging.Infof(ctx, "cipd: registering %s", instance.Pin())
	result, err := client.remote.registerInstance(ctx, instance.Pin())
	if err != nil {
		return err
	}

	// Asked to upload the package file to CAS first?
	if result.uploadSession != nil {
		err = client.UploadToCAS(
			ctx, instance.Pin().InstanceID, instance.DataReader(),
			result.uploadSession, timeout)
		if err != nil {
			return err
		}
		// Try again, now that file is uploaded.
		logging.Infof(ctx, "cipd: registering %s", instance.Pin())
		result, err = client.remote.registerInstance(ctx, instance.Pin())
		if err != nil {
			return err
		}
		if result.uploadSession != nil {
			return ErrBadUpload
		}
	}

	if result.alreadyRegistered {
		logging.Infof(
			ctx, "cipd: instance %s is already registered by %s on %s",
			instance.Pin(), result.registeredBy, result.registeredTs)
	} else {
		logging.Infof(ctx, "cipd: instance %s was successfully registered", instance.Pin())
	}

	return nil
}

func (client *clientImpl) DeletePackage(ctx context.Context, packageName string) error {
	if err := common.ValidatePackageName(packageName); err != nil {
		return err
	}
	return client.remote.deletePackage(ctx, packageName)
}

func (client *clientImpl) IncrementCounter(ctx context.Context, pin common.Pin, counter string, delta int) error {
	return client.remote.incrementCounter(ctx, pin, counter, delta)
}

func (client *clientImpl) ReadCounter(ctx context.Context, pin common.Pin, counter string) (Counter, error) {
	return client.remote.readCounter(ctx, pin, counter)
}

func (client *clientImpl) SetRefWhenReady(ctx context.Context, ref string, pin common.Pin) error {
	if err := common.ValidatePackageRef(ref); err != nil {
		return err
	}
	if err := common.ValidatePin(pin); err != nil {
		return err
	}
	logging.Infof(ctx, "cipd: setting ref of %q: %q => %q", pin.PackageName, ref, pin.InstanceID)
	deadline := clock.Now(ctx).Add(SetRefTimeout)
	for clock.Now(ctx).Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := client.remote.setRef(ctx, ref, pin)
		if err == nil {
			return nil
		}
		if _, ok := err.(*pendingProcessingError); ok {
			logging.Warningf(ctx, "cipd: package instance is not ready yet - %s", err)
			clock.Sleep(ctx, 5*time.Second)
		} else {
			logging.Errorf(ctx, "cipd: failed to set ref - %s", err)
			return err
		}
	}
	logging.Errorf(ctx, "cipd: failed set ref - deadline exceeded")
	return ErrSetRefTimeout
}

func (client *clientImpl) AttachTagsWhenReady(ctx context.Context, pin common.Pin, tags []string) error {
	err := common.ValidatePin(pin)
	if err != nil {
		return err
	}
	if len(tags) == 0 {
		return nil
	}
	for _, tag := range tags {
		logging.Infof(ctx, "cipd: attaching tag %s", tag)
	}
	deadline := clock.Now(ctx).Add(TagAttachTimeout)
	for clock.Now(ctx).Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		err = client.remote.attachTags(ctx, pin, tags)
		if err == nil {
			logging.Infof(ctx, "cipd: all tags attached")
			return nil
		}
		if _, ok := err.(*pendingProcessingError); ok {
			logging.Warningf(ctx, "cipd: package instance is not ready yet - %s", err)
			clock.Sleep(ctx, 5*time.Second)
		} else {
			logging.Errorf(ctx, "cipd: failed to attach tags - %s", err)
			return err
		}
	}
	logging.Errorf(ctx, "cipd: failed to attach tags - deadline exceeded")
	return ErrAttachTagsTimeout
}

func (client *clientImpl) SearchInstances(ctx context.Context, tag, packageName string) (common.PinSlice, error) {
	if packageName != "" {
		// Don't bother searching if packageName is invalid.
		if err := common.ValidatePackageName(packageName); err != nil {
			return common.PinSlice{}, err
		}
	}
	return client.remote.searchInstances(ctx, tag, packageName)
}

func (client *clientImpl) ListInstances(ctx context.Context, packageName string) (InstanceEnumerator, error) {
	if err := common.ValidatePackageName(packageName); err != nil {
		return nil, err
	}
	return &instanceEnumeratorImpl{
		fetch: func(ctx context.Context, limit int, cursor string) ([]InstanceInfo, string, error) {
			resp, err := client.remote.listInstances(ctx, packageName, limit, cursor)
			if err != nil {
				return nil, "", err
			}
			return resp.instances, resp.cursor, nil
		},
	}, nil
}

func (client *clientImpl) FetchInstanceInfo(ctx context.Context, pin common.Pin) (InstanceInfo, error) {
	err := common.ValidatePin(pin)
	if err != nil {
		return InstanceInfo{}, err
	}
	info, err := client.remote.fetchInstance(ctx, pin)
	if err != nil {
		return InstanceInfo{}, err
	}
	return InstanceInfo{
		Pin:          pin,
		RegisteredBy: info.registeredBy,
		RegisteredTs: UnixTime(info.registeredTs),
	}, nil
}

type sortByTagKey []TagInfo

func (s sortByTagKey) Len() int      { return len(s) }
func (s sortByTagKey) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s sortByTagKey) Less(i, j int) bool {
	k1 := instanceTagKey(s[i].Tag)
	k2 := instanceTagKey(s[j].Tag)
	if k1 == k2 {
		// Newest first.
		return s[j].RegisteredTs.Before(s[i].RegisteredTs)
	}
	return k1 < k2
}

// instanceTagKey returns key portion of the instance tag or empty string.
func instanceTagKey(t string) string {
	chunks := strings.SplitN(t, ":", 2)
	if len(chunks) != 2 {
		return ""
	}
	return chunks[0]
}

func (client *clientImpl) FetchInstanceTags(ctx context.Context, pin common.Pin, tags []string) ([]TagInfo, error) {
	if err := common.ValidatePin(pin); err != nil {
		return nil, err
	}
	fetched, err := client.remote.fetchTags(ctx, pin, tags)
	if err != nil {
		return nil, err
	}
	sort.Sort(sortByTagKey(fetched))
	return fetched, nil
}

func (client *clientImpl) FetchInstanceRefs(ctx context.Context, pin common.Pin, refs []string) ([]RefInfo, error) {
	if err := common.ValidatePin(pin); err != nil {
		return nil, err
	}
	return client.remote.fetchRefs(ctx, pin, refs)
}

func (client *clientImpl) FetchPackageRefs(ctx context.Context, packageName string) ([]RefInfo, error) {
	if err := common.ValidatePackageName(packageName); err != nil {
		return nil, err
	}
	out, err := client.remote.fetchPackage(ctx, packageName, true)
	if err != nil {
		return nil, err
	}
	return out.refs, nil
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
// hash along the way.
func (client *clientImpl) remoteFetchInstance(ctx context.Context, pin common.Pin, output io.WriteSeeker) (err error) {
	defer func() {
		if err != nil {
			logging.Errorf(ctx, "cipd: failed to fetch %s - %s", pin, err)
		} else {
			logging.Infof(ctx, "cipd: successfully fetched %s", pin)
		}
	}()

	logging.Infof(ctx, "cipd: resolving fetch URL for %s", pin)
	fetchInfo, err := client.remote.fetchInstance(ctx, pin)
	if err != nil {
		return
	}
	hash, err := local.HashForInstanceID(pin.InstanceID)
	if err != nil {
		return
	}
	if err = client.storage.download(ctx, fetchInfo.fetchURL, output, hash); err != nil {
		return
	}
	if local.InstanceIDFromHash(hash) != pin.InstanceID {
		err = fmt.Errorf("package hash mismatch")
	}
	return
}

func (client *clientImpl) FetchAndDeployInstance(ctx context.Context, subdir string, pin common.Pin) error {
	if err := common.ValidateSubdir(subdir); err != nil {
		return err
	}
	if err := common.ValidatePin(pin); err != nil {
		return err
	}

	doit := func() error {
		// Fetch the package (verifying its hash) and obtain a pointer to its data.
		instanceFile, err := client.FetchInstance(ctx, pin)
		if err != nil {
			return err
		}

		defer func() {
			corrupt := local.IsCorruptionError(err)
			if err := instanceFile.Close(ctx, corrupt); err != nil && err != os.ErrClosed {
				logging.Warningf(ctx, "cipd: failed to close the package file - %s", err)
			}
		}()

		// Open the instance. This reads its manifest. 'FetchInstance' has verified
		// the hash already, so skip verification.
		instance, err := local.OpenInstance(ctx, instanceFile, pin.InstanceID, local.SkipHashVerification)
		if err != nil {
			return err
		}

		// Opportunistically clean up trashed files.
		defer client.doBatchAwareOp(ctx, batchAwareOpCleanupTrash)

		// Deploy it. 'defer' will take care of removing the temp file if needed.
		_, err = client.deployer.DeployInstance(ctx, subdir, instance)
		return err
	}

	err := doit()
	if err != nil && local.IsCorruptionError(err) {
		logging.WithError(err).Warningf(ctx, "cipd: unpacking failed, retrying.")
		err = doit()
	}
	return err
}

func (client *clientImpl) EnsurePackages(ctx context.Context, allPins common.PinSliceBySubdir, dryRun bool) (aMap ActionMap, err error) {
	if err = allPins.Validate(); err != nil {
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
	aMap = buildActionPlan(allPins, existing)
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

	// Install all new and updated stuff. Install in the order specified by
	// 'pins'. Order matters if multiple packages install same file.
	aMap.LoopOrdered(func(subdir string, actions *Actions) {
		toDeploy := make(map[string]bool, len(actions.ToInstall)+len(actions.ToUpdate))
		for _, p := range actions.ToInstall {
			toDeploy[p.PackageName] = true
		}
		for _, pair := range actions.ToUpdate {
			toDeploy[pair.To.PackageName] = true
		}
		for _, pin := range allPins[subdir] {
			if !toDeploy[pin.PackageName] {
				continue
			}
			err = client.FetchAndDeployInstance(ctx, subdir, pin)
			if err != nil {
				logging.Errorf(ctx, "Failed to install %s - %s", pin, err)
				hasErrors = true
				actions.Errors = append(actions.Errors, ActionError{
					Action: "install",
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

////////////////////////////////////////////////////////////////////////////////
// Private structs and interfaces.

type remote interface {
	fetchACL(ctx context.Context, packagePath string) ([]PackageACL, error)
	modifyACL(ctx context.Context, packagePath string, changes []PackageACLChange) error

	resolveVersion(ctx context.Context, packageName, version string) (common.Pin, error)

	initiateUpload(ctx context.Context, sha1 string) (*UploadSession, error)
	finalizeUpload(ctx context.Context, sessionID string) (bool, error)
	registerInstance(ctx context.Context, pin common.Pin) (*registerInstanceResponse, error)

	fetchPackage(ctx context.Context, packageName string, withRefs bool) (*fetchPackageResponse, error)
	deletePackage(ctx context.Context, packageName string) error

	setRef(ctx context.Context, ref string, pin common.Pin) error
	attachTags(ctx context.Context, pin common.Pin, tags []string) error
	fetchTags(ctx context.Context, pin common.Pin, tags []string) ([]TagInfo, error)
	fetchRefs(ctx context.Context, pin common.Pin, refs []string) ([]RefInfo, error)
	fetchInstance(ctx context.Context, pin common.Pin) (*fetchInstanceResponse, error)
	fetchClientBinaryInfo(ctx context.Context, pin common.Pin) (*fetchClientBinaryInfoResponse, error)

	listPackages(ctx context.Context, path string, recursive, showHidden bool) ([]string, []string, error)
	searchInstances(ctx context.Context, tag, packageName string) (common.PinSlice, error)
	listInstances(ctx context.Context, packageName string, limit int, cursor string) (*listInstancesResponse, error)

	incrementCounter(ctx context.Context, pin common.Pin, counter string, delta int) error
	readCounter(ctx context.Context, pin common.Pin, counter string) (Counter, error)
}

type storage interface {
	upload(ctx context.Context, url string, data io.ReadSeeker) error
	download(ctx context.Context, url string, output io.WriteSeeker, h hash.Hash) error
}

type fetchPackageResponse struct {
	registeredBy string
	registeredTs time.Time
	hidden       bool

	refs []RefInfo
}

type registerInstanceResponse struct {
	uploadSession     *UploadSession
	alreadyRegistered bool
	registeredBy      string
	registeredTs      time.Time
}

type fetchInstanceResponse struct {
	fetchURL     string
	registeredBy string
	registeredTs time.Time
}

type clientBinary struct {
	FileName string `json:"file_name"`
	SHA1     string `json:"sha1"`
	FetchURL string `json:"fetch_url"`
	Size     int64  `json:"size,string"`
}

type fetchClientBinaryInfoResponse struct {
	instance     *InstanceInfo
	clientBinary *clientBinary
}

type listInstancesResponse struct {
	instances []InstanceInfo
	cursor    string
}

// deleteOnClose deletes the file once it is closed.
type deleteOnClose struct {
	*os.File
}

func (d deleteOnClose) Close(ctx context.Context, corrupt bool) (err error) {
	name := d.File.Name()
	defer func() {
		if rmErr := os.Remove(name); err == nil && rmErr != nil && !os.IsNotExist(rmErr) {
			err = rmErr
		}
	}()
	return d.File.Close()
}

// UnderlyingFile is only used by tests and shouldn't be used directly.
func (d deleteOnClose) UnderlyingFile() *os.File {
	return d.File
}

// Private stuff.

// buildActionPlan is used by EnsurePackages to figure out what to install or remove.
func buildActionPlan(desired, existing common.PinSliceBySubdir) (aMap ActionMap) {
	desiredSubdirs := stringset.New(len(desired))
	for desiredSubdir := range desired {
		desiredSubdirs.Add(desiredSubdir)
	}

	existingSubdirs := stringset.New(len(existing))
	for existingSubdir := range existing {
		existingSubdirs.Add(existingSubdir)
	}

	aMap = ActionMap{}

	// all newly added subdirs
	desiredSubdirs.Difference(existingSubdirs).Iter(func(subdir string) bool {
		if want := desired[subdir]; len(want) > 0 {
			aMap[subdir] = &Actions{ToInstall: want}
		}
		return true
	})

	// all removed subdirs
	existingSubdirs.Difference(desiredSubdirs).Iter(func(subdir string) bool {
		if have := existing[subdir]; len(have) > 0 {
			aMap[subdir] = &Actions{ToRemove: have}
		}
		return true
	})

	// all common subdirs
	desiredSubdirs.Intersect(existingSubdirs).Iter(func(subdir string) bool {
		a := Actions{}

		// Figure out what needs to be installed or updated.
		haveMap := existing[subdir].ToMap()
		for _, want := range desired[subdir] {
			if haveID, exists := haveMap[want.PackageName]; !exists {
				a.ToInstall = append(a.ToInstall, want)
			} else if haveID != want.InstanceID {
				a.ToUpdate = append(a.ToUpdate, UpdatedPin{
					From: common.Pin{PackageName: want.PackageName, InstanceID: haveID},
					To:   want,
				})
			}
		}

		// Figure out what needs to be removed.
		wantMap := desired[subdir].ToMap()
		for _, have := range existing[subdir] {
			if wantMap[have.PackageName] == "" {
				a.ToRemove = append(a.ToRemove, have)
			}
		}

		if !a.Empty() {
			aMap[subdir] = &a
		}
		return true
	})

	if len(aMap) == 0 {
		return nil
	}
	return
}

type instanceEnumeratorImpl struct {
	// A user-supplied callback that fetches the next page of results.
	fetch func(ctx context.Context, limit int, cursor string) (out []InstanceInfo, nextCursor string, err error)

	cursor string // last fetched cursor or "" at the start of the fetch
	done   bool   // true if fetched the last page
}

// Next returns next up to 'limit' instances or 0 if there's no more.
func (e *instanceEnumeratorImpl) Next(ctx context.Context, limit int) (out []InstanceInfo, err error) {
	if e.done {
		return nil, nil
	}
	out, nextCursor, err := e.fetch(ctx, limit, e.cursor)
	if err != nil {
		return nil, err
	}
	e.cursor = nextCursor
	e.done = nextCursor == ""
	return
}
