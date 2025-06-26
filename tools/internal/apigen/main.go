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

package apigen

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"text/template"
	"time"

	"go.chromium.org/luci/common/clock"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/sync/parallel"
)

const (
	defaultPackageBase = "go.chromium.org/luci/common/api"

	// chromiumLicence is the standard Chromium license header.
	chromiumLicense = `` +
		"// Copyright {{.Year}} The LUCI Authors.\n" +
		"//\n" +
		"// Licensed under the Apache License, Version 2.0 (the \"License\");\n" +
		"// you may not use this file except in compliance with the License.\n" +
		"// You may obtain a copy of the License at\n" +
		"//\n" +
		"//      http://www.apache.org/licenses/LICENSE-2.0\n" +
		"//\n" +
		"// Unless required by applicable law or agreed to in writing, software\n" +
		"// distributed under the License is distributed on an \"AS IS\" BASIS,\n" +
		"// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.\n" +
		"// See the License for the specific language governing permissions and\n" +
		"// limitations under the License.\n" +
		"\n"
)

var (
	// chromiumLicenseTemplate is the compiled Chromium license template text.
	chromiumLicenseTemplate = template.Must(template.New("chromium license").Parse(chromiumLicense))

	// apiGoGenLicenseHdr is a start of a comment block with license header put by
	// google-api-go-generator, which we remove and replace with chromium license.
	apiGoGenLicenseHdr = regexp.MustCompile("// Copyright [0-9]+ Google.*")
)

func compileChromiumLicense(c context.Context) (string, error) {
	buf := bytes.Buffer{}
	err := chromiumLicenseTemplate.Execute(&buf, map[string]any{
		"Year": clock.Now(c).Year(),
	})
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Application is the main apigen application instance.
type Application struct {
	servicePath    string
	serviceAPIRoot string
	genPath        string
	apiPackage     string
	apiSubproject  string
	apiAllowlist   apiAllowlist
	baseURL        string

	license string
}

// AddToFlagSet adds application-level flags to the supplied FlagSet.
func (a *Application) AddToFlagSet(fs *flag.FlagSet) {
	flag.StringVar(&a.servicePath, "service", ".",
		"Path to the AppEngine service to generate from.")
	flag.StringVar(&a.serviceAPIRoot, "service-api-root", "/_ah/api/",
		"The service's API root path.")
	flag.StringVar(&a.genPath, "generator", "google-api-go-generator",
		"Path to the `google-api-go-generator` binary to use.")
	flag.StringVar(&a.apiPackage, "api-package", defaultPackageBase,
		"Name of the root API package on GOPATH.")
	flag.StringVar(&a.apiSubproject, "api-subproject", "",
		"If supplied, place APIs in an additional subdirectory under -api-package.")
	flag.Var(&a.apiAllowlist, "api",
		"If supplied, limit the emitted APIs to those named. Can be specified "+
			"multiple times.")
	flag.StringVar(&a.baseURL, "base-url", "http://localhost:8080",
		"Use this as the default base service client URL.")
}

func resolveExecutable(path *string) error {
	if path == nil || *path == "" {
		return errors.New("empty path")
	}
	lpath, err := exec.LookPath(*path)
	if err != nil {
		return fmt.Errorf("could not find [%s]: %s", *path, err)
	}

	st, err := os.Stat(lpath)
	if err != nil {
		return err
	}
	if st.Mode().Perm()&0111 == 0 {
		return errors.New("file is not executable")
	}
	*path = lpath
	return nil
}

// retryHTTP executes an HTTP call to the specified URL, retrying if it fails.
//
// It will return an error if no successful HTTP results were returned.
// Otherwise, it will return the body of the successful HTTP response.
func retryHTTP(c context.Context, u url.URL, method, body string) ([]byte, error) {
	client := http.Client{}

	gen := func() retry.Iterator {
		return &retry.Limited{
			Delay:   2 * time.Second,
			Retries: 20,
		}
	}

	output := []byte(nil)
	err := retry.Retry(c, gen, func() error {
		req := http.Request{
			Method: method,
			URL:    &u,
			Header: http.Header{},
		}
		if len(body) > 0 {
			req.Body = io.NopCloser(bytes.NewBuffer([]byte(body)))
			req.ContentLength = int64(len(body))
			req.Header.Add("Content-Type", "application/json")
		}

		resp, err := client.Do(&req)
		if err != nil {
			return err
		}
		if resp.Body != nil {
			defer resp.Body.Close()
			output, err = io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
		}

		switch resp.StatusCode {
		case http.StatusOK, http.StatusNoContent:
			return nil

		default:
			return fmt.Errorf("unsuccessful status code (%d): %s", resp.StatusCode, resp.Status)
		}
	}, func(err error, d time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"url":        u.String(),
			"delay":      d,
		}.Infof(c, "Service is not up yet; retrying.")
	})
	if err != nil {
		return nil, err
	}

	log.Fields{
		"url": u.String(),
	}.Infof(c, "Service is alive!")
	return output, nil
}

// Run executes the application using the supplied context.
//
// Note that this intentionally consumes the Application by value, as we may
// modify its configuration as parameters become resolved.
func (a Application) Run(c context.Context) error {
	if err := resolveExecutable(&a.genPath); err != nil {
		return fmt.Errorf("invalid API generator path (-google-api-go-generator): %s", err)
	}

	apiDst, err := getPackagePath(a.apiPackage)
	if err != nil {
		return fmt.Errorf("failed to find package path for [%s]: %s", a.apiPackage, err)
	}
	if a.apiSubproject != "" {
		apiDst = augPath(apiDst, a.apiSubproject)
		a.apiPackage = strings.Join([]string{a.apiPackage, a.apiSubproject}, "/")
	}
	log.Fields{
		"package": a.apiPackage,
		"path":    apiDst,
	}.Debugf(c, "Identified API destination package path.")

	// Compile our Chromium license.
	a.license, err = compileChromiumLicense(c)
	if err != nil {
		return fmt.Errorf("failed to compile Chromium license: %s", err)
	}

	c, cancelFunc := context.WithCancel(c)
	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, os.Interrupt)
	go func() {
		for range sigC {
			cancelFunc()
		}
	}()
	defer signal.Stop(sigC)

	// (1) Execute our service. Capture its discovery API.
	svc, err := loadService(c, a.servicePath)
	if err != nil {
		return fmt.Errorf("failed to load service [%s]: %s", a.servicePath, err)
	}

	err = svc.run(c, func(c context.Context, discoveryURL url.URL) error {
		discoveryURL.Path = safeURLPathJoin(discoveryURL.Path, a.serviceAPIRoot, "discovery", "v1", "apis")

		data, err := retryHTTP(c, discoveryURL, "GET", "")
		if err != nil {
			return fmt.Errorf("discovery server did not come online: %s", err)
		}

		dir := directoryList{}
		if err := json.Unmarshal(data, &dir); err != nil {
			return fmt.Errorf("failed to load directory list: %s", err)
		}

		// Ensure that our target API base directory exists.
		if err := ensureDirectory(apiDst); err != nil {
			return fmt.Errorf("failed to create destination directory: %s", err)
		}

		// Run "google-api-go-generator" against the hosted service.
		err = parallel.FanOutIn(func(taskC chan<- func() error) {
			for i, item := range dir.Items {
				c := log.SetFields(c, log.Fields{
					"index": i,
					"api":   item.ID,
				})

				if !a.isAllowed(item.ID) {
					log.Infof(c, "API is not requested; skipping.")
					continue
				}

				taskC <- func() error {
					return a.generateAPI(c, item, &discoveryURL, apiDst)
				}
			}
		})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to extract APIs.")
	}

	return nil
}

// generateAPI generates and installs a single directory item's API.
func (a *Application) generateAPI(c context.Context, item *directoryItem, discoveryURL *url.URL, dst string) error {
	tmpdir, err := os.MkdirTemp(os.TempDir(), "apigen")
	if err != nil {
		return err
	}
	defer func() {
		os.RemoveAll(tmpdir)
	}()

	gendir := augPath(tmpdir, "gen")
	headerPath := augPath(tmpdir, "header.txt")
	if err := os.WriteFile(headerPath, []byte(a.license), 0644); err != nil {
		return err
	}

	args := []string{
		"-cache=false", // Apparently the form {"-cache", "false"} is ignored.
		"-discoveryurl", discoveryURL.String(),
		"-api", item.ID,
		"-gendir", gendir,
		"-api_pkg_base", a.apiPackage,
		"-base_url", a.baseURL,
		"-header_path", headerPath,
	}
	log.Fields{
		"command": a.genPath,
		"args":    args,
	}.Debugf(c, "Executing google-api-go-generator.")
	out, err := exec.Command(a.genPath, args...).CombinedOutput()
	log.Infof(c, "Output:\n%s", out)
	if err != nil {
		return fmt.Errorf("error executing google-api-go-generator: %s", err)
	}

	err = installSource(gendir, dst, func(relpath string, data []byte) ([]byte, error) {
		// Skip the root "api-list.json" file. This is generated only for the subset
		// of APIs that this installation is handling, and is not representative of
		// the full discovery (much less installation) API set.
		if relpath == "api-list.json" {
			return nil, nil
		}

		if !strings.HasSuffix(relpath, "-gen.go") {
			return data, nil
		}

		log.Fields{
			"relpath": relpath,
		}.Infof(c, "Fixing up generated Go file.")

		// Remove copyright header added by google-api-go-generator. We have our own
		// already.
		filtered := strings.Builder{}
		alreadySkippedHeader := false
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			if line := scanner.Text(); alreadySkippedHeader || !apiGoGenLicenseHdr.MatchString(line) {
				// Use a vendored copy of "google.golang.org/api/internal/gensupport", since
				// we can't refer to the internal one. See crbug.com/1003496.
				line = strings.ReplaceAll(line,
					`"google.golang.org/api/internal/gensupport"`,
					`"go.chromium.org/luci/common/api/internal/gensupport"`)
				// This is forbidden. We'll replace symbols imported from it.
				if line == "\tinternal \"google.golang.org/api/internal\"" {
					continue
				}
				// This is the only symbol imported from `internal`.
				line = strings.ReplaceAll(line, "internal.Version", "\"luci-go\"")
				// Finish writing the line to the output.
				filtered.WriteString(line)
				filtered.WriteRune('\n')
				continue
			}

			// Found the start of the comment block with the header. Skip it all.
			for scanner.Scan() {
				if line := scanner.Text(); !strings.HasPrefix(line, "//") {
					// The comment block is usually followed by an empty line which we
					// also skip. But be careful in case it's not.
					if line != "" {
						filtered.WriteString(line)
						filtered.WriteRune('\n')
					}
					break
				}
			}

			// Carry on copying the rest of lines unchanged.
			alreadySkippedHeader = true
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return []byte(filtered.String()), nil
	})
	if err != nil {
		return fmt.Errorf("failed to install [%s]: %s", item.ID, err)
	}
	return nil
}

func (a *Application) isAllowed(id string) bool {
	if len(a.apiAllowlist) == 0 {
		return true
	}
	for _, w := range a.apiAllowlist {
		if w == id {
			return true
		}
	}
	return false
}

func safeURLPathJoin(p ...string) string {
	for i, v := range p {
		p[i] = strings.Trim(v, "/")
	}
	return strings.Join(p, "/")
}
