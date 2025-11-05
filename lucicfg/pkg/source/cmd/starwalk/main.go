// Copyright 2025 The LUCI Authors.
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

// Command starwalk implements a simple tool which can grab the starfiles and
// their input of a specific git tree from a remote repo and put it onto disk
// using a gitsource Cache.
//
// This is intended as an experimental vehicle for the gitsource package while
// developing it (to ensure that it actually works against real remotes).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"go.starlark.net/syntax"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"

	"go.chromium.org/luci/lucicfg/pkg/source"
	"go.chromium.org/luci/lucicfg/pkg/source/gitilessource"
	"go.chromium.org/luci/lucicfg/pkg/source/gitsource"
)

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

var (
	cacheRoot  = flag.String("cache-root", "", "(required) Path to cache root.")
	remoteUrl  = flag.String("remote", "", "(required) URL of git remote.")
	ref        = flag.String("ref", "", "(required) Ref in the git remote (e.g. refs/heads/XXX).")
	commit     = flag.String("commit", "", "(required) The commit to extract from.")
	pkgRoot    = flag.String("pkg-root", "", "(required) The root directory of the lucicfg package.")
	entrypoint = flag.String("entrypoint", "", "(required) Path to star-file entrypoint.")
	output     = flag.String("output", "", "(required) Path to the output. Output must be empty or not exist.")
	dumpOne    = flag.String("dump-one", "", "(optional) just get all files @ tree, then dump this one.")
	verbose    = flag.Bool("verbose", false, "(optional) turn on verbose logs.")
	pickNewest stringlistflag.Flag
	authFlags  authcli.Flags
)

func init() {
	flag.Var(&pickNewest, "pick-newest", "additional commits to pick the newest from, in addition to commit")
	authOpts := chromeinfra.DefaultAuthOptions()
	authOpts.Scopes = []string{
		auth.OAuthScopeEmail,
		gitiles.OAuthScope,
	}
	authFlags.Register(flag.CommandLine, authOpts)
}

func cast[T syntax.Node](n syntax.Node) (ret T) {
	casted, ok := n.(T)
	if !ok {
		return
	}
	return casted
}

func isIdentCall(call *syntax.CallExpr, path ...string) *syntax.CallExpr {
	// If we want `a.b.c(...)` then we must find:
	//
	// CallExpr{
	//   Fn: DotExpr{  // (a.b).c
	//     X: DotExpr {  // a.b
	//       X: Ident{a}
	//       Name: Ident{b}
	//     }
	//     Name: Ident{c}
	//   }
	//   ...
	// }
	nameTree := call.Fn

	// peel off DotExpr's
	for len(path) > 1 {
		// we expect to see a DotExpr with Name == path[-1]
		dotExp := cast[*syntax.DotExpr](nameTree)
		if dotExp == nil {
			return nil
		}
		if dotExp.Name.Name != path[len(path)-1] {
			return nil
		}
		path = path[:len(path)-1]
		nameTree = dotExp.X
	}

	// expect to find Ident{path[0]}
	ident := cast[*syntax.Ident](nameTree)
	if ident == nil {
		return nil
	}
	if ident.Name != path[0] {
		return nil
	}
	return call
}

func makePkgRel(filename, target string) string {
	if strings.HasPrefix(target, "//") {
		return target[2:]
	}
	if strings.HasPrefix(target, "@") {
		return ""
	}
	return path.Join(path.Dir(filename), target)
}

func quickParse(filename string, data []byte) (ret []string) {
	parsed := must((&syntax.FileOptions{
		Set: true,
	}).Parse(filename, data, 0))
	// looking for:
	//   - load statements (easy)
	//   - exec statements (easy)
	//   - io.read_file(<path>)
	//   - io.read_proto_file(ignore, <path>, ignore)

	addLiteralString := func(node syntax.Node) {
		lit := cast[*syntax.Literal](node)
		if lit == nil {
			return
		}
		if strval, ok := lit.Value.(string); ok {
			if pkgRel := makePkgRel(filename, strval); pkgRel != "" {
				ret = append(ret, pkgRel)
			}
		}
	}

	for _, statement := range parsed.Stmts {
		// walk the statement to see if it has `io.file_read` or
		// `io.read_proto_file`
		syntax.Walk(statement, func(n syntax.Node) bool {
			switch x := n.(type) {
			case *syntax.LoadStmt:
				// load(module, ...)
				addLiteralString(x.Module)
				return false

			case *syntax.CallExpr:
				if strings.HasSuffix(filename, "project.star") {
					if dxn := cast[*syntax.DotExpr](x.Fn); dxn != nil {
						fmt.Printf("%#v\n", n)
						fmt.Printf(".Fn.X: %#v\n", dxn.X)
						fmt.Printf(".Fn.Name: %#v\n", dxn.Name)
					}
				}

				// exec(module)
				if call := isIdentCall(x, "exec"); call != nil {
					addLiteralString(call.Args[0])
					return false
				}
				if call := isIdentCall(x, "io", "read_file"); call != nil {
					addLiteralString(call.Args[0])
					return false
				}
				if call := isIdentCall(x, "io", "read_proto"); call != nil {
					addLiteralString(call.Args[1])
					return false
				}
			}
			return true
		})
	}

	return ret
}

func main() {
	flag.Parse()

	ctx := context.Background()
	ctx = gologger.StdConfig.Use(ctx)
	ctx = logging.SetLevel(ctx, logging.Debug)

	cacheRootAbs := must(filepath.Abs(*cacheRoot))

	cache := must(gitilessource.New(
		filepath.Join(cacheRootAbs, "gitiles"), must(authFlags.Options()),
		must(gitsource.New(filepath.Join(cacheRootAbs, "git"), *verbose))))

	repo := must(cache.ForRepo(ctx, *remoteUrl))

	if len(pickNewest) > 0 {
		newest := must(repo.PickMostRecent(ctx, *ref, pickNewest))
		fmt.Println(newest)
		return
	}

	if *dumpOne != "" {
		fetcher := must(repo.Fetcher(ctx, *ref, *commit, *pkgRoot, func(kind source.ObjectKind, pkgRelPath string) bool {
			return true
		}))
		dumped := must(fetcher.Read(ctx, *dumpOne))
		must(os.Stdout.Write(dumped))
		return
	}

	patterns := []string{".json", ".cfg", ".template", ".star"}

	fetcher := must(repo.Fetcher(ctx, *ref, *commit, *pkgRoot, func(kind source.ObjectKind, pkgRelPath string) (prefetch bool) {
		if kind == source.TreeKind && strings.HasSuffix(pkgRelPath, "/generated") || pkgRelPath == "generated" {
			return false
		}
		if kind != source.BlobKind {
			return true // walk all other trees
		}
		for _, suffix := range patterns {
			if strings.HasSuffix(pkgRelPath, suffix) {
				return true
			}
		}
		return false
	}))

	processed := stringset.New(10)
	toWalk := []string{*entrypoint}

	for len(toWalk) > 0 {
		item := toWalk[0]
		toWalk = toWalk[1:]
		processed.Add(item)
		logging.Debugf(ctx, "processing: %q", item)

		data := must(fetcher.Read(ctx, item))
		outPath := filepath.Join(*output, item)
		if err := os.MkdirAll(filepath.Dir(outPath), 0777); err != nil {
			panic(err)
		}
		if err := os.WriteFile(outPath, data, 0666); err != nil {
			panic(err)
		}
		logging.Debugf(ctx, "wrote: %q", outPath)

		if strings.HasSuffix(item, ".star") {
			for _, pth := range quickParse(item, data) {
				if !processed.Has(pth) {
					toWalk = append(toWalk, pth)
				}
			}
		}
	}
}
