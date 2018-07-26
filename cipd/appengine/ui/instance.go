// Copyright 2018 The LUCI Authors.
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

package ui

import (
	"sort"
	"strings"
	"sync"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl"
	"go.chromium.org/luci/cipd/common"
)

func instancePage(c *router.Context, pkg, ver string) error {
	pkg = strings.Trim(pkg, "/")
	if err := common.ValidatePackageName(pkg); err != nil {
		return status.Errorf(codes.InvalidArgument, "bad package name - %s", err)
	}
	pfx := ""
	if i := strings.LastIndex(pkg, "/"); i != -1 {
		pfx = pkg[:i]
	}

	// In parallel: resolve the version, and find all siblings. Showing siblings
	// is extremely useful for per-platform packages, e.g. pkg/name/linux-amd64.
	inst, siblings, err := resolveVersionAndFindSiblings(c.Context, pfx, pkg, ver)
	if err != nil {
		return err
	}

	// Do the rest in parallel. There can be only transient errors returned here,
	// so collect them all into single Internal error.
	var resolvedSiblings map[string]*resolvedOrErr
	var desc *api.DescribeInstanceResponse
	var url *api.ObjectURL
	err = parallel.FanOutIn(func(tasks chan<- func() error) {
		// Find siblings that have this version too. Skip this if the version is
		// given as instance ID, since siblings can't have exact same instance ID
		// as 'inst'.
		if common.ValidateInstanceID(ver, common.AnyHash) != nil {
			tasks <- func() (err error) {
				resolvedSiblings, err = resolveSiblingsVersion(c.Context, siblings, ver)
				return
			}
		} else {
			resolvedSiblings = make(map[string]*resolvedOrErr, len(siblings))
			for _, s := range siblings {
				resolvedSiblings[s] = &resolvedOrErr{
					Package: s,
					Missing: true,
					Skipped: true,
				}
			}
		}

		// Find all tags and refs attached to the instance in question.
		tasks <- func() (err error) {
			desc, err = impl.PublicRepo.DescribeInstance(c.Context, &api.DescribeInstanceRequest{
				Package:            inst.Package,
				Instance:           inst.Instance,
				DescribeRefs:       true,
				DescribeTags:       true,
				DescribeProcessors: true,
			})
			return
		}

		// Grab a download URL, so users can download the package as *.zip. Note
		// that ACLs have already been checked by ResolveVersion.
		tasks <- func() (err error) {
			url, err = impl.InternalCAS.GetObjectURL(c.Context, &api.GetObjectURLRequest{
				Object:           inst.Instance,
				DownloadFilename: common.ObjectRefToInstanceID(inst.Instance) + ".zip",
			})
			return
		}
	})
	if err != nil {
		return status.Errorf(codes.Internal, "%s", err)
	}

	// Construct the final sorted list of siblings for display purposes. This list
	// also includes 'pkg' itself now as well.
	family := make([]*resolvedOrErr, 0, len(siblings)+1)
	family = append(family, &resolvedOrErr{
		Package:  pkg,
		Instance: inst,
		Current:  true,
	})
	for _, r := range resolvedSiblings {
		family = append(family, r)
	}
	for _, r := range family {
		r.Title = r.Package[strings.LastIndex(r.Package, "/")+1:]
		r.Version = ver
		r.PackageHref = packagePageURL(r.Package, "")
		r.InstanceHref = instancePageURL(r.Package, ver)
	}
	sort.Slice(family, func(i, j int) bool {
		return family[i].Package < family[j].Package
	})

	templates.MustRender(c.Context, c.Writer, "pages/instance.html", map[string]interface{}{
		"Prefix":      pfx,
		"Package":     pkg,
		"Instance":    inst,
		"Version":     ver,
		"Breadcrumbs": breadcrumbs(pkg, ver),
		"Family":      family,
	})
	return nil
}

////////////////////////////////////////////////////////////////////////////////

// resolveVersionAndFindSiblings resolves 'ver' to an instance ID, and finds
// all siblings of 'pkg' (in parallel).
//
// Checks correctness of 'pkg' and 'ver' and checks ACLs as a side effect.
func resolveVersionAndFindSiblings(ctx context.Context, pfx, pkg, ver string) (inst *api.Instance, siblings []string, err error) {
	// Errors are returned through separate vars. Easier than fishing them out of
	// MultiError returned by FanOutIn, considered we care about ResolveVersion
	// error in particular.
	var instErr error
	var siblingsErr error
	parallel.FanOutIn(func(tasks chan<- func() error) {
		tasks <- func() error {
			inst, instErr = impl.PublicRepo.ResolveVersion(ctx, &api.ResolveVersionRequest{
				Package: pkg,
				Version: ver,
			})
			return instErr
		}
		tasks <- func() error {
			var listing *api.ListPrefixResponse
			listing, siblingsErr = impl.PublicRepo.ListPrefix(ctx, &api.ListPrefixRequest{
				Prefix: pfx,
			})
			if listing != nil {
				// Skip 'pkg' itself. It's not a sibling.
				siblings = listing.Packages[:0]
				for _, s := range listing.Packages {
					if s != pkg {
						siblings = append(siblings, s)
					}
				}
			}
			return siblingsErr
		}
	})

	// Prefer to return ResolveVersion error, it is more descriptive.
	switch {
	case instErr != nil:
		err = instErr
	case siblingsErr != nil:
		err = siblingsErr
	}
	return
}

////////////////////////////////////////////////////////////////////////////////

type resolvedOrErr struct {
	Instance *api.Instance
	Err      string

	// Fields below are used from templates.

	Package      string // set even if Instance is nil
	Title        string // last component of the package name
	Version      string // always same as 'ver' passed to the instancePage()
	PackageHref  string // always set, URL of the package page
	InstanceHref string // set only if resolved the version

	Current bool // true if this entry matches the instance we are viewing
	Missing bool // true if there's no such version
	Broken  bool // true if some other fatal error happen during resolution
	Skipped bool // true (additionally to Missing) if 'ver' was instance ID
}

// resolveSiblingsVersion resolves 'ver' into a concrete package instance for
// all packages in 'pkgs', returning them as a map 'pkg name -> resolvedOrErr'.
//
// Returns only transient errors. All fatal errors (e.g. ambiguity in
// resolution) is returned through resolvedOrErr state.
func resolveSiblingsVersion(ctx context.Context, pkgs []string, ver string) (out map[string]*resolvedOrErr, err error) {
	out = make(map[string]*resolvedOrErr, len(pkgs))
	err = parallel.FanOutIn(func(tasks chan<- func() error) {
		m := sync.Mutex{}
		for _, pkg := range pkgs {
			pkg := pkg

			tasks <- func() error {
				inst, err := impl.PublicRepo.ResolveVersion(ctx, &api.ResolveVersionRequest{
					Package: pkg,
					Version: ver,
				})

				entry := &resolvedOrErr{Package: pkg}
				switch status.Code(err) {
				case codes.Internal, codes.Unknown:
					return err
				case codes.OK:
					entry.Instance = inst
				case codes.NotFound:
					entry.Missing = true
				default:
					entry.Broken = true
					if s, ok := status.FromError(err); ok {
						entry.Err = s.Message()
					} else {
						entry.Err = err.Error()
					}
				}

				m.Lock()
				out[pkg] = entry
				m.Unlock()
				return nil // fatal errors are returned through 'resolvedOrErr'
			}
		}
	})
	return
}
