// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

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
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"

	"github.com/luci/luci-go/client/cipd/common"
	"github.com/luci/luci-go/client/cipd/internal"
	"github.com/luci/luci-go/client/cipd/local"
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

	// UserAgent is HTTP user agent string for CIPD client.
	UserAgent = "cipd 1.1"

	// ServiceURL is URL of a backend to connect to by default.
	ServiceURL = "https://chrome-infra-packages.appspot.com"
)

var (
	// ErrFinalizationTimeout is returned if CAS service can not finalize upload fast enough.
	ErrFinalizationTimeout = errors.WrapTransient(errors.New("timeout while waiting for CAS service to finalize the upload"))
	// ErrBadUpload is returned when a package file is uploaded, but servers asks us to upload it again.
	ErrBadUpload = errors.WrapTransient(errors.New("package file is uploaded, but servers asks us to upload it again"))
	// ErrBadUploadSession is returned by UploadToCAS if provided UploadSession is not valid.
	ErrBadUploadSession = errors.New("uploadURL must be set if UploadSessionID is used")
	// ErrUploadSessionDied is returned by UploadToCAS if upload session suddenly disappeared.
	ErrUploadSessionDied = errors.WrapTransient(errors.New("upload session is unexpectedly missing"))
	// ErrNoUploadSessionID is returned by UploadToCAS if server didn't provide upload session ID.
	ErrNoUploadSessionID = errors.New("server didn't provide upload session ID")
	// ErrSetRefTimeout is returned when service refuses to move a ref for a long time.
	ErrSetRefTimeout = errors.WrapTransient(errors.New("timeout while moving a ref"))
	// ErrAttachTagsTimeout is returned when service refuses to accept tags for a long time.
	ErrAttachTagsTimeout = errors.WrapTransient(errors.New("timeout while attaching tags"))
	// ErrDownloadError is returned by FetchInstance on download errors.
	ErrDownloadError = errors.WrapTransient(errors.New("failed to download the package file after multiple attempts"))
	// ErrUploadError is returned by RegisterInstance and UploadToCAS on upload errors.
	ErrUploadError = errors.WrapTransient(errors.New("failed to upload the package file after multiple attempts"))
	// ErrAccessDenined is returned by calls talking to backend on 401 or 403 HTTP errors.
	ErrAccessDenined = errors.New("access denied (not authenticated or not enough permissions)")
	// ErrBackendInaccessible is returned by calls talking to backed if it doesn't response.
	ErrBackendInaccessible = errors.WrapTransient(errors.New("request to the backend failed after multiple attempts"))
	// ErrEnsurePackagesFailed is returned by EnsurePackages if something is not right.
	ErrEnsurePackagesFailed = errors.New("failed to update packages, see the log")
)

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

// MarshalJSON is used by JSON encoder.
func (t UnixTime) MarshalJSON() ([]byte, error) {
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

// InstanceInfo is returned by FetchInstanceInfo.
type InstanceInfo struct {
	// Pin identifies package instance.
	Pin common.Pin `json:"pin"`
	// RegisteredBy is identify of whoever uploaded this instance.
	RegisteredBy string `json:"registered_by"`
	// RegisteredTs is when the instance was registered.
	RegisteredTs UnixTime `json:"registered_ts"`
}

// TagInfo is returned by FetchInstanceTags.
type TagInfo struct {
	// Tag is actual tag name ("key:value" pair).
	Tag string `json:"tag"`
	// RegisteredBy is identify of whoever attached this tag.
	RegisteredBy string `json:"registered_by"`
	// RegisteredTs is when the tag was registered.
	RegisteredTs UnixTime `json:"registered_ts"`
}

// RefInfo is returned by FetchInstanceRefs.
type RefInfo struct {
	// Ref is the ref name.
	Ref string `json:"ref"`
	// ModifiedBy is identify of whoever modified this ref last time.
	ModifiedBy string `json:"modified_by"`
	// ModifiedTs is when the ref was modified last time.
	ModifiedTs UnixTime `json:"modified_ts"`
}

// Actions is returned by EnsurePackages. It lists pins that were attempted to
// be installed, updated or removed, as well as all errors.
type Actions struct {
	ToInstall []common.Pin  `json:"to_install,omitempty"` // pins to be installed
	ToUpdate  []UpdatedPin  `json:"to_update,omitempty"`  // pins to be replaced
	ToRemove  []common.Pin  `json:"to_remove,omitempty"`  // pins to be removed
	Errors    []ActionError `json:"errors,omitempty"`     // all individual errors
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

// Client provides high-level CIPD client interface. Thread safe.
type Client interface {
	// FetchACL returns a list of PackageACL objects (parent paths first) that
	// together define the access control list for the given package subpath.
	FetchACL(packagePath string) ([]PackageACL, error)

	// ModifyACL applies a set of PackageACLChanges to a package path.
	ModifyACL(packagePath string, changes []PackageACLChange) error

	// UploadToCAS uploads package data blob to Content Addressed Store if it is
	// not there already. The data is addressed by SHA1 hash (also known as
	// package's InstanceID). It can be used as a standalone function (if
	// 'session' is nil) or as a part of more high level upload process (in that
	// case upload session can be opened elsewhere and its properties passed here
	// via 'session' argument). Returns nil on successful upload.
	UploadToCAS(sha1 string, data io.ReadSeeker, session *UploadSession, timeout time.Duration) error

	// ResolveVersion converts an instance ID, a tag or a ref into a concrete Pin
	// by contacting the backend.
	ResolveVersion(packageName, version string) (common.Pin, error)

	// RegisterInstance makes the package instance available for clients by
	// uploading it to the storage and registering it in the package repository.
	//
	// 'instance' is a package instance to register.
	// 'timeout' is how long to wait for backend-side package hash
	//     verification to succeed (pass zero for some sensible default).
	RegisterInstance(instance local.PackageInstance, timeout time.Duration) error

	// SetRefWhenReady moves a ref to point to a package instance, retrying on
	// "not yet processed" responses.
	SetRefWhenReady(ref string, pin common.Pin) error

	// AttachTagsWhenReady attaches tags to an instance, retrying on "not yet
	// processed" responses.
	AttachTagsWhenReady(pin common.Pin, tags []string) error

	// FetchInstanceInfo returns general information about the instance, such as
	// who registered it and when.
	FetchInstanceInfo(pin common.Pin) (InstanceInfo, error)

	// FetchInstanceTags returns information about tags attached to the package
	// instance sorted by tag key and creation timestamp (newest first). If 'tags'
	// is empty, fetches all attached tags, otherwise only ones specified.
	FetchInstanceTags(pin common.Pin, tags []string) ([]TagInfo, error)

	// FetchInstanceRefs returns information about refs pointing to the package
	// instance sorted by modification timestamp (newest first). If 'ref' is
	// empty, fetches all refs, otherwise only ones specified.
	FetchInstanceRefs(pin common.Pin, refs []string) ([]RefInfo, error)

	// FetchInstance downloads package instance file from the repository.
	FetchInstance(pin common.Pin, output io.WriteSeeker) error

	// FetchAndDeployInstance fetches the package instance and deploys it into
	// the site root. It doesn't check whether the instance is already deployed.
	FetchAndDeployInstance(pin common.Pin) error

	// ListPackages returns a list of strings of package names.
	ListPackages(path string, recursive bool) ([]string, error)

	// SearchInstances finds all instances with given tag and optionally name and
	// returns their concrete Pins.
	SearchInstances(tag, packageName string) ([]common.Pin, error)

	// ProcessEnsureFile parses text file that describes what should be installed
	// by EnsurePackages function. It is a text file where each line has a form:
	// <package name> <desired version>. Whitespaces are ignored. Lines that start
	// with '#' are ignored. Version can be specified as instance ID, tag or ref.
	// Will resolve tags and refs to concrete instance IDs by calling the backend.
	ProcessEnsureFile(r io.Reader) ([]common.Pin, error)

	// EnsurePackages is high-level interface for installation, removal and update
	// of packages inside the installation site root. Given a description of
	// what packages (and versions) should be installed it will do all necessary
	// actions to bring the state of the site root to the desired one.
	//
	// If dryRun is true, will just check for changes and return them in Actions
	// struct, but won't actually perform them.
	//
	// If the update was only partially applied, returns both Actions and error.
	EnsurePackages(pins []common.Pin, dryRun bool) (Actions, error)

	// Close should be called to dump any cached state to disk.
	Close()
}

// HTTPClientFactory lazily creates http.Client to use for making requests.
type HTTPClientFactory func() (*http.Client, error)

// ClientOptions is passed to NewClient factory function.
type ClientOptions struct {
	// ServiceURL is root URL of the backend service.
	ServiceURL string

	// Root is a site root directory (a directory where packages will be
	// installed to). It also hosts .cipd/* directory that tracks internal state
	// of installed packages and keeps various cache files. 'Root' can be an empty
	// string if the client is not going to be used to deploy or remove local
	// packages. If both Root and CacheDir are empty, tag cache is disabled.
	Root string

	// Logger is a logger to use for logs (null-logger by default).
	Logger logging.Logger

	// AuthenticatedClientFactory lazily creates http.Client to use for making
	// RPC requests.
	AuthenticatedClientFactory HTTPClientFactory

	// AnonymousClientFactory lazily creates http.Client to use for making
	// requests to storage.
	AnonymousClientFactory HTTPClientFactory

	// UserAgent is put into User-Agent HTTP header with each request.
	UserAgent string

	// CacheDir is a directory for shared cache. If empty, tags are cached
	// inside the site root. If both Root and CacheDir are empty, tag cache
	// is disabled.
	CacheDir string
}

// NewClient initializes CIPD client object.
func NewClient(opts ClientOptions) Client {
	if opts.ServiceURL == "" {
		opts.ServiceURL = ServiceURL
	}
	if opts.Logger == nil {
		opts.Logger = logging.Null()
	}
	if opts.AnonymousClientFactory == nil {
		opts.AnonymousClientFactory = func() (*http.Client, error) { return http.DefaultClient, nil }
	}
	if opts.AuthenticatedClientFactory == nil {
		opts.AuthenticatedClientFactory = opts.AnonymousClientFactory
	}
	if opts.UserAgent == "" {
		opts.UserAgent = UserAgent
	}
	c := &clientImpl{
		ClientOptions: opts,
		clock:         &clockImpl{},
	}
	c.remote = &remoteImpl{c}
	c.storage = &storageImpl{c, uploadChunkSize}
	c.deployer = local.NewDeployer(opts.Root, opts.Logger)
	return c
}

type clientImpl struct {
	ClientOptions

	// lock protects lazily initialized portions of the client.
	lock sync.Mutex

	// clock provides current time and ability to sleep. Thread safe.
	clock clock

	// remote knows how to call backend REST API. Thread safe.
	remote remote

	// storage knows how to upload and download raw binaries using signed URLs.
	// Thread safe.
	storage storage

	// deployer knows how to install packages to local file system. Thread safe.
	deployer local.Deployer

	// tagCacheLock is used to synchronize access to the tag cache file.
	tagCacheLock sync.Mutex

	// authClient is a lazily created http.Client to use for authenticated
	// requests. Thread safe, but lazily initialized under lock.
	authClient *http.Client

	// anonClient is a lazily created http.Client to use for anonymous requests.
	// Thread safe, but lazily initialized under lock.
	anonClient *http.Client
}

// doAuthenticatedHTTPRequest is used by remote implementation to make HTTP calls.
func (client *clientImpl) doAuthenticatedHTTPRequest(req *http.Request) (*http.Response, error) {
	return client.doRequest(req, &client.authClient, client.AuthenticatedClientFactory)
}

// doAnonymousHTTPRequest is used by storage implementation to make HTTP calls.
func (client *clientImpl) doAnonymousHTTPRequest(req *http.Request) (*http.Response, error) {
	return client.doRequest(req, &client.anonClient, client.AnonymousClientFactory)
}

// doRequest lazy-initializes http.Client using provided factory and then
// executes the request.
func (client *clientImpl) doRequest(req *http.Request, c **http.Client, fac HTTPClientFactory) (*http.Response, error) {
	httpClient, err := func() (*http.Client, error) {
		client.lock.Lock()
		defer client.lock.Unlock()
		var err error
		if *c == nil {
			*c, err = fac()
		}
		return *c, err
	}()
	if err != nil {
		return nil, err
	}
	return httpClient.Do(req)
}

// tagCachePath returns path to a tag cache file or "" if tag cache is disabled.
func (client *clientImpl) tagCachePath() string {
	var dir string
	switch {
	case client.CacheDir != "":
		dir = client.CacheDir

	case client.Root != "":
		dir = filepath.Join(client.Root, local.SiteServiceDir)

	default:
		return ""
	}

	return filepath.Join(dir, "tagcache.db")
}

// withTagCache checks if tag cache is enabled; if yes, loads it, calls f and
// saves back if it was modified.
// Calls are serialized.
func (client *clientImpl) withTagCache(f func(*internal.TagCache)) {
	path := client.tagCachePath()
	if path == "" {
		return
	}

	client.tagCacheLock.Lock()
	defer client.tagCacheLock.Unlock()

	start := time.Now()
	cache, err := internal.LoadTagCacheFromFile(path)
	if err != nil {
		client.Logger.Warningf("cipd: failed to load tag cache - %s", err)
		cache = &internal.TagCache{}
	}
	loadSaveTime := time.Since(start)

	f(cache)

	if cache.Dirty() {
		// It's tiny in size (and protobuf can't serialize to io.Reader anyway). Dump
		// it to disk via FileSystem object to deal with possible concurrent updates,
		// missing directories, etc.
		fs := local.NewFileSystem(filepath.Dir(path), client.Logger)
		start = time.Now()
		out, err := cache.Save()
		if err == nil {
			err = fs.EnsureFile(path, bytes.NewReader(out))
		}
		loadSaveTime += time.Since(start)
		if err != nil {
			client.Logger.Warningf("cipd: failed to update tag cache - %s", err)
		}
	}

	if loadSaveTime > time.Second {
		client.Logger.Warningf("cipd: loading and saving tag cache with %d entries took %s", cache.Len(), loadSaveTime)
	}
}

func (client *clientImpl) FetchACL(packagePath string) ([]PackageACL, error) {
	return client.remote.fetchACL(packagePath)
}

func (client *clientImpl) ModifyACL(packagePath string, changes []PackageACLChange) error {
	return client.remote.modifyACL(packagePath, changes)
}

func (client *clientImpl) ListPackages(path string, recursive bool) ([]string, error) {
	pkgs, dirs, err := client.remote.listPackages(path, recursive)
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

func (client *clientImpl) UploadToCAS(sha1 string, data io.ReadSeeker, session *UploadSession, timeout time.Duration) error {
	// Open new upload session if an existing is not provided.
	var err error
	if session == nil {
		client.Logger.Infof("cipd: uploading %s: initiating", sha1)
		session, err = client.remote.initiateUpload(sha1)
		if err != nil {
			client.Logger.Warningf("cipd: can't upload %s - %s", sha1, err)
			return err
		}
		if session == nil {
			client.Logger.Infof("cipd: %s is already uploaded", sha1)
			return nil
		}
	} else {
		if session.ID == "" || session.URL == "" {
			return ErrBadUploadSession
		}
	}

	// Upload the file to CAS storage.
	err = client.storage.upload(session.URL, data)
	if err != nil {
		return err
	}

	// Finalize the upload, wait until server verifies and publishes the file.
	if timeout == 0 {
		timeout = CASFinalizationTimeout
	}
	started := client.clock.now()
	delay := time.Second
	for {
		published, err := client.remote.finalizeUpload(session.ID)
		if err != nil {
			client.Logger.Warningf("cipd: upload of %s failed: %s", sha1, err)
			return err
		}
		if published {
			client.Logger.Infof("cipd: successfully uploaded %s", sha1)
			return nil
		}
		if client.clock.now().Sub(started) > timeout {
			client.Logger.Warningf("cipd: upload of %s failed: timeout", sha1)
			return ErrFinalizationTimeout
		}
		client.Logger.Infof("cipd: uploading - verifying")
		client.clock.sleep(delay)
		if delay < 4*time.Second {
			delay += 500 * time.Millisecond
		}
	}
}

func (client *clientImpl) ResolveVersion(packageName, version string) (common.Pin, error) {
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
	isTag := common.ValidateInstanceTag(version) == nil
	if isTag {
		var cached common.Pin
		client.withTagCache(func(tc *internal.TagCache) {
			cached = tc.ResolveTag(packageName, version)
		})
		if cached.InstanceID != "" {
			client.Logger.Debugf("cipd: tag cache hit for %s:%s - %s", packageName, version, cached.InstanceID)
			return cached, nil
		}
	}
	pin, err := client.remote.resolveVersion(packageName, version)
	if err != nil {
		return pin, err
	}
	if isTag {
		client.withTagCache(func(tc *internal.TagCache) {
			tc.AddTag(pin, version)
		})
	}
	return pin, nil
}

func (client *clientImpl) RegisterInstance(instance local.PackageInstance, timeout time.Duration) error {
	// Attempt to register.
	client.Logger.Infof("cipd: registering %s", instance.Pin())
	result, err := client.remote.registerInstance(instance.Pin())
	if err != nil {
		return err
	}

	// Asked to upload the package file to CAS first?
	if result.uploadSession != nil {
		err = client.UploadToCAS(instance.Pin().InstanceID, instance.DataReader(), result.uploadSession, timeout)
		if err != nil {
			return err
		}
		// Try again, now that file is uploaded.
		client.Logger.Infof("cipd: registering %s", instance.Pin())
		result, err = client.remote.registerInstance(instance.Pin())
		if err != nil {
			return err
		}
		if result.uploadSession != nil {
			return ErrBadUpload
		}
	}

	if result.alreadyRegistered {
		client.Logger.Infof(
			"cipd: instance %s is already registered by %s on %s",
			instance.Pin(), result.registeredBy, result.registeredTs)
	} else {
		client.Logger.Infof("cipd: instance %s was successfully registered", instance.Pin())
	}

	return nil
}

func (client *clientImpl) SetRefWhenReady(ref string, pin common.Pin) error {
	if err := common.ValidatePackageRef(ref); err != nil {
		return err
	}
	if err := common.ValidatePin(pin); err != nil {
		return err
	}
	client.Logger.Infof("cipd: setting ref of %q: %q => %q", pin.PackageName, ref, pin.InstanceID)
	deadline := client.clock.now().Add(SetRefTimeout)
	for client.clock.now().Before(deadline) {
		err := client.remote.setRef(ref, pin)
		if err == nil {
			return nil
		}
		if _, ok := err.(*pendingProcessingError); ok {
			client.Logger.Warningf("cipd: package instance is not ready yet - %s", err)
			client.clock.sleep(5 * time.Second)
		} else {
			client.Logger.Errorf("cipd: failed to set ref - %s", err)
			return err
		}
	}
	client.Logger.Errorf("cipd: failed set ref - deadline exceeded")
	return ErrSetRefTimeout
}

func (client *clientImpl) AttachTagsWhenReady(pin common.Pin, tags []string) error {
	err := common.ValidatePin(pin)
	if err != nil {
		return err
	}
	if len(tags) == 0 {
		return nil
	}
	for _, tag := range tags {
		client.Logger.Infof("cipd: attaching tag %s", tag)
	}
	deadline := client.clock.now().Add(TagAttachTimeout)
	for client.clock.now().Before(deadline) {
		err = client.remote.attachTags(pin, tags)
		if err == nil {
			client.Logger.Infof("cipd: all tags attached")
			return nil
		}
		if _, ok := err.(*pendingProcessingError); ok {
			client.Logger.Warningf("cipd: package instance is not ready yet - %s", err)
			client.clock.sleep(5 * time.Second)
		} else {
			client.Logger.Errorf("cipd: failed to attach tags - %s", err)
			return err
		}
	}
	client.Logger.Errorf("cipd: failed to attach tags - deadline exceeded")
	return ErrAttachTagsTimeout
}

func (client *clientImpl) SearchInstances(tag, packageName string) ([]common.Pin, error) {
	if packageName != "" {
		// Don't bother searching if packageName is invalid.
		if err := common.ValidatePackageName(packageName); err != nil {
			return []common.Pin{}, err
		}
	}
	return client.remote.searchInstances(tag, packageName)
}

func (client *clientImpl) FetchInstanceInfo(pin common.Pin) (InstanceInfo, error) {
	err := common.ValidatePin(pin)
	if err != nil {
		return InstanceInfo{}, err
	}
	info, err := client.remote.fetchInstance(pin)
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
	k1 := common.GetInstanceTagKey(s[i].Tag)
	k2 := common.GetInstanceTagKey(s[j].Tag)
	if k1 == k2 {
		// Newest first.
		return s[j].RegisteredTs.Before(s[i].RegisteredTs)
	}
	return k1 < k2
}

func (client *clientImpl) FetchInstanceTags(pin common.Pin, tags []string) ([]TagInfo, error) {
	err := common.ValidatePin(pin)
	if err != nil {
		return nil, err
	}
	fetched, err := client.remote.fetchTags(pin, tags)
	if err != nil {
		return nil, err
	}
	sort.Sort(sortByTagKey(fetched))
	return fetched, nil
}

func (client *clientImpl) FetchInstanceRefs(pin common.Pin, refs []string) ([]RefInfo, error) {
	err := common.ValidatePin(pin)
	if err != nil {
		return nil, err
	}
	return client.remote.fetchRefs(pin, refs)
}

func (client *clientImpl) FetchInstance(pin common.Pin, output io.WriteSeeker) error {
	err := common.ValidatePin(pin)
	if err != nil {
		return err
	}
	client.Logger.Infof("cipd: resolving fetch URL for %s", pin)
	fetchInfo, err := client.remote.fetchInstance(pin)
	if err == nil {
		err = client.storage.download(fetchInfo.fetchURL, output)
	}
	if err != nil {
		client.Logger.Errorf("cipd: failed to fetch %s - %s", pin, err)
		return err
	}
	client.Logger.Infof("cipd: successfully fetched %s", pin)
	return nil
}

func (client *clientImpl) FetchAndDeployInstance(pin common.Pin) error {
	err := common.ValidatePin(pin)
	if err != nil {
		return err
	}

	// Use temp file for storing package file. Delete it when done.
	var instance local.PackageInstance
	f, err := client.deployer.TempFile(pin.InstanceID)
	if err != nil {
		return err
	}
	defer func() {
		// Instance takes ownership of the file, no need to close it separately.
		if instance == nil {
			f.Close()
		}
		os.Remove(f.Name())
	}()

	// Fetch the package data to the provided storage.
	err = client.FetchInstance(pin, f)
	if err != nil {
		return err
	}

	// Open the instance, verify the instance ID.
	instance, err = local.OpenInstance(f, pin.InstanceID)
	if err != nil {
		return err
	}
	defer instance.Close()

	// Deploy it. 'defer' will take care of removing the temp file if needed.
	_, err = client.deployer.DeployInstance(instance)
	return err
}

func (client *clientImpl) ProcessEnsureFile(r io.Reader) ([]common.Pin, error) {
	lineNo := 0
	makeError := func(msg string) error {
		return fmt.Errorf("failed to parse desired state (line %d): %s", lineNo, msg)
	}

	out := []common.Pin{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lineNo++

		// Split each line into words, ignore white space.
		tokens := []string{}
		for _, chunk := range strings.Split(scanner.Text(), " ") {
			chunk = strings.TrimSpace(chunk)
			if chunk != "" {
				tokens = append(tokens, chunk)
			}
		}

		// Skip empty lines or lines starting with '#'.
		if len(tokens) == 0 || tokens[0][0] == '#' {
			continue
		}

		// Each line has a format "<package name> <version>".
		if len(tokens) != 2 {
			return nil, makeError("expecting '<package name> <version>' line")
		}
		err := common.ValidatePackageName(tokens[0])
		if err != nil {
			return nil, makeError(err.Error())
		}
		err = common.ValidateInstanceVersion(tokens[1])
		if err != nil {
			return nil, makeError(err.Error())
		}

		// Good enough.
		pin, err := client.ResolveVersion(tokens[0], tokens[1])
		if err != nil {
			return nil, err
		}
		out = append(out, pin)
	}

	return out, nil
}

func (client *clientImpl) EnsurePackages(pins []common.Pin, dryRun bool) (actions Actions, err error) {
	// Make sure a package is specified only once.
	seen := make(map[string]bool, len(pins))
	for _, p := range pins {
		if seen[p.PackageName] {
			return actions, fmt.Errorf("package %s is specified twice", p.PackageName)
		}
		seen[p.PackageName] = true
	}

	// Enumerate existing packages.
	existing, err := client.deployer.FindDeployed()
	if err != nil {
		return actions, err
	}

	// Figure out what needs to be updated and deleted, log it.
	actions = buildActionPlan(pins, existing)
	if actions.Empty() {
		client.Logger.Debugf("Everything is up-to-date.")
		return actions, nil
	}
	if len(actions.ToInstall) != 0 {
		client.Logger.Infof("Packages to be installed:")
		for _, pin := range actions.ToInstall {
			client.Logger.Infof("  %s", pin)
		}
	}
	if len(actions.ToUpdate) != 0 {
		client.Logger.Infof("Packages to be updated:")
		for _, pair := range actions.ToUpdate {
			client.Logger.Infof("  %s (%s -> %s)",
				pair.From.PackageName, pair.From.InstanceID, pair.To.InstanceID)
		}
	}
	if len(actions.ToRemove) != 0 {
		client.Logger.Infof("Packages to be removed:")
		for _, pin := range actions.ToRemove {
			client.Logger.Infof("  %s", pin)
		}
	}

	if dryRun {
		client.Logger.Infof("Dry run, not actually doing anything.")
		return actions, nil
	}

	// Remove all unneeded stuff.
	for _, pin := range actions.ToRemove {
		err = client.deployer.RemoveDeployed(pin.PackageName)
		if err != nil {
			client.Logger.Errorf("Failed to remove %s - %s", pin.PackageName, err)
			actions.Errors = append(actions.Errors, ActionError{
				Action: "remove",
				Pin:    pin,
				Error:  JSONError{err},
			})
		}
	}

	// Install all new and updated stuff. Install in the order specified by
	// 'pins'. Order matters if multiple packages install same file.
	toDeploy := make(map[string]bool, len(actions.ToInstall)+len(actions.ToUpdate))
	for _, p := range actions.ToInstall {
		toDeploy[p.PackageName] = true
	}
	for _, pair := range actions.ToUpdate {
		toDeploy[pair.To.PackageName] = true
	}
	for _, pin := range pins {
		if !toDeploy[pin.PackageName] {
			continue
		}
		err = client.FetchAndDeployInstance(pin)
		if err != nil {
			client.Logger.Errorf("Failed to install %s - %s", pin, err)
			actions.Errors = append(actions.Errors, ActionError{
				Action: "install",
				Pin:    pin,
				Error:  JSONError{err},
			})
		}
	}

	if len(actions.Errors) == 0 {
		client.Logger.Infof("All changes applied.")
		return actions, nil
	}
	return actions, ErrEnsurePackagesFailed
}

func (client *clientImpl) Close() {
	client.lock.Lock()
	defer client.lock.Unlock()
	client.authClient = nil
	client.anonClient = nil
}

////////////////////////////////////////////////////////////////////////////////
// Private structs and interfaces.

type clock interface {
	now() time.Time
	sleep(time.Duration)
}

type remote interface {
	fetchACL(packagePath string) ([]PackageACL, error)
	modifyACL(packagePath string, changes []PackageACLChange) error

	resolveVersion(packageName, version string) (common.Pin, error)

	initiateUpload(sha1 string) (*UploadSession, error)
	finalizeUpload(sessionID string) (bool, error)
	registerInstance(pin common.Pin) (*registerInstanceResponse, error)

	setRef(ref string, pin common.Pin) error
	attachTags(pin common.Pin, tags []string) error
	fetchTags(pin common.Pin, tags []string) ([]TagInfo, error)
	fetchRefs(pin common.Pin, refs []string) ([]RefInfo, error)
	fetchInstance(pin common.Pin) (*fetchInstanceResponse, error)

	listPackages(path string, recursive bool) ([]string, []string, error)
	searchInstances(tag, packageName string) ([]common.Pin, error)
}

type storage interface {
	upload(url string, data io.ReadSeeker) error
	download(url string, output io.WriteSeeker) error
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

// Private stuff.

type clockImpl struct{}

func (c *clockImpl) now() time.Time        { return time.Now() }
func (c *clockImpl) sleep(d time.Duration) { time.Sleep(d) }

// buildActionPlan is used by EnsurePackages to figure out what to install or remove.
func buildActionPlan(desired, existing []common.Pin) (a Actions) {
	// Figure out what needs to be installed or updated.
	existingMap := buildInstanceIDMap(existing)
	for _, d := range desired {
		if existingID, exists := existingMap[d.PackageName]; !exists {
			a.ToInstall = append(a.ToInstall, d)
		} else if existingID != d.InstanceID {
			a.ToUpdate = append(a.ToUpdate, UpdatedPin{
				From: common.Pin{PackageName: d.PackageName, InstanceID: existingID},
				To:   d,
			})
		}
	}

	// Figure out what needs to be removed.
	desiredMap := buildInstanceIDMap(desired)
	for _, e := range existing {
		if desiredMap[e.PackageName] == "" {
			a.ToRemove = append(a.ToRemove, e)
		}
	}

	return
}

// buildInstanceIDMap builds mapping {package name -> instance ID}.
func buildInstanceIDMap(pins []common.Pin) map[string]string {
	out := map[string]string{}
	for _, p := range pins {
		out[p.PackageName] = p.InstanceID
	}
	return out
}
