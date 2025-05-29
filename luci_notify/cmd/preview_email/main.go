// Copyright 2019 The LUCI Authors.
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

// Command preview_email renders an email template file.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/protobuf/jsonpb"

	"go.chromium.org/luci/buildbucket/cli"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"

	"go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/mailtmpl"
)

type parsedFlags struct {
	TemplateRootDir     string
	BuildbucketHostname string
	OldStatus           buildbucketpb.Status
}

func main() {
	ctx := context.Background()

	f := parsedFlags{
		OldStatus: buildbucketpb.Status_SUCCESS,
	}

	flag.StringVar(&f.TemplateRootDir, "template-root-dir", "", text.Doc(`
		Path to the email template dir.
		Defaults to the parent directory of the template file
	`))
	flag.Var(cli.StatusFlag(&f.OldStatus), "old-status", text.Doc(`
		Previous status of the builder.
	`))
	flag.StringVar(&f.BuildbucketHostname, "buildbucket-hostname", chromeinfra.BuildbucketHost, "Buildbucket hostname")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), text.Doc(`
			Usage: preview_email TEMPLATE_FILE [BUILD]

			BUILD is a path to a buildbucket.v2.Build JSON file
			https://chromium.googlesource.com/infra/luci/luci-go/+/HEAD/buildbucket/proto/build.proto
			If not provided, reads the build JSON from stdin.

			TEMPLATE_FILE is a path to a email template file.

			Example: fetch a live build using bb tool and render an email for it
				bb get -json -A 8914184822697034512 | preview_email ./default.template
		`))
		flag.PrintDefaults()
	}

	flag.Parse()

	var buildPath, templatePath string
	switch len(flag.Args()) {
	case 2:
		buildPath = flag.Arg(1)
		fallthrough
	case 1:
		templatePath = flag.Arg(0)
	default:
		flag.Usage()
		os.Exit(1)
	}

	if err := run(ctx, templatePath, buildPath, f); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, templateFile, buildPath string, f parsedFlags) error {
	build, err := readBuild(buildPath)
	if err != nil {
		return errors.Fmt("failed to read build: %w", err)
	}

	if templateFile, err = filepath.Abs(templateFile); err != nil {
		return err
	}
	if _, err := os.Stat(templateFile); err != nil {
		return err
	}

	if f.TemplateRootDir == "" {
		f.TemplateRootDir = filepath.Dir(templateFile)
	} else if f.TemplateRootDir, err = filepath.Abs(f.TemplateRootDir); err != nil {
		return err
	}

	bundle := readTemplateBundle(ctx, f.TemplateRootDir)
	templateName := templateName(templateFile, f.TemplateRootDir)
	subject, body := bundle.GenerateEmail(templateName, &config.TemplateInput{
		BuildbucketHostname: f.BuildbucketHostname,
		Build:               build,
		OldStatus:           f.OldStatus,
	})

	fmt.Println(subject)
	fmt.Println()
	fmt.Println(body)
	return nil
}

func readBuild(buildPath string) (*buildbucketpb.Build, error) {
	var f *os.File
	if buildPath == "" {
		f = os.Stdin
	} else {
		var err error
		f, err = os.Open(buildPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
	}

	build := &buildbucketpb.Build{}
	return build, jsonpb.Unmarshal(f, build)
}

func readTemplateBundle(ctx context.Context, templateRootDir string) *mailtmpl.Bundle {
	templateRootDir, err := filepath.Abs(templateRootDir)
	if err != nil {
		return &mailtmpl.Bundle{Err: err}
	}

	var templates []*mailtmpl.Template
	err = filepath.Walk(templateRootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, mailtmpl.FileExt) {
			return err
		}

		t := &mailtmpl.Template{
			Name: templateName(path, templateRootDir),
			// Note: path is absolute.
			DefinitionURL: "file://" + filepath.ToSlash(path),
		}

		contents, err := os.ReadFile(path)
		if err != nil {
			return errors.Fmt("failed to read %q: %w", path, err)
		}

		t.SubjectTextTemplate, t.BodyHTMLTemplate, err = mailtmpl.SplitTemplateFile(string(contents))
		if err != nil {
			return errors.Fmt("failed to parse %q: %w", path, err)
		}

		templates = append(templates, t)
		return nil
	})

	b := mailtmpl.NewBundle(templates)
	if b.Err == nil {
		b.Err = err
	}
	return b
}

func templateName(templateFile, templateRootDir string) string {
	templateFile = filepath.ToSlash(strings.TrimPrefix(templateFile, templateRootDir))
	templateFile = strings.TrimPrefix(templateFile, "/")
	return strings.TrimSuffix(templateFile, mailtmpl.FileExt)
}
