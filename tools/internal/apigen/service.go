// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apigen

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"

	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
	"gopkg.in/yaml.v2"
)

// appYAML is a subset of the contents of an AppEngine application's "app.yaml"
// descriptor neeeded by this service.
type appYAML struct {
	Runtime string `yaml:"runtime"`
	VM      bool   `yaml:"vm"`
}

type service interface {
	run(context.Context, serviceRunFunc) error
}

type serviceRunFunc func(context.Context, url.URL) error

type serviceConfig struct {
	path    string
	port    int
	apiRoot string
}

// loadService is a generic service loader routine. It attempts to:
// 1) Identify the filesystem path of the service being described.
// 2) Analyze its "app.yaml" to determine its runtime parameters.
// 3) Construct and return a `service` instance for the result.
//
// "path" is first treated as a filesystem path, then considered as a Go package
// path.
func (sc *serviceConfig) loadService(c context.Context) (service, error) {
	path := sc.path
	yamlPath := ""
	st, err := os.Stat(path)
	switch {
	case os.IsNotExist(err):
		log.Fields{
			"path": path,
		}.Debugf(c, "Path does not exist. Maybe it's a Go path?")

		// Not a filesysem path. Perhaps it's a Go package on GOPATH?
		pkgPath, err := getPackagePath(path)
		if err != nil {
			log.Fields{
				"path": path,
			}.Debugf(c, "Could not resolve package path.")
			return nil, fmt.Errorf("could not resolve path [%s]", path)
		}
		path = pkgPath

	case err != nil:
		return nil, fmt.Errorf("failed to stat [%s]: %s", path, err)

	case st.IsDir():
		break

	default:
		// "path" is a path to a non-directory. Use its parent directory.
		yamlPath, err = filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("could not get absolute path for YAML config [%s]: %s", path, err)
		}
		path = filepath.Dir(path)
	}

	// "path" is a directory. Does its `app.yaml` exist?
	if yamlPath == "" {
		yamlPath = filepath.Join(path, "app.yaml")
	}

	if _, err = os.Stat(yamlPath); err != nil {
		return nil, fmt.Errorf("unable to stat YAML config at [%s]: %s", yamlPath, err)
	}

	configData, err := ioutil.ReadFile(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML config at [%s]: %s", yamlPath, err)
	}

	config := appYAML{}
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to Unmarshal YAML config from [%s]: %s", yamlPath, err)
	}

	switch config.Runtime {
	case "go":
		if config.VM {
			return &discoveryTranslateService{
				serviceConfig: *sc,
				dir:           path,
			}, nil
		}
		return &devAppserverService{
			args: []string{"goapp", "serve", yamlPath},
		}, nil

	case "python27":
		return &devAppserverService{
			args: []string{"dev_appserver.py", yamlPath},
		}, nil

	default:
		return nil, fmt.Errorf("don't know how to load service runtime [%s]", config.Runtime)
	}
}

type devAppserverService struct {
	args []string
}

func (s *devAppserverService) run(c context.Context, f serviceRunFunc) error {
	log.Fields{
		"args": s.args,
	}.Infof(c, "Executing service.")

	if len(s.args) == 0 {
		return errors.New("no command configured")
	}

	// Execute `dev_appserver`.
	cmd := &killableCommand{
		Cmd: exec.Command(s.args[0], s.args[1:]...),
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer cmd.kill(c)

	return f(c, url.URL{
		Scheme: "http",
		Host:   "localhost:8080", // Default dev_appserver.py host/port.
		Path:   "/_ah/api/discovery/v1/apis",
	})
}

// discoveryTranslateService is a service that loads a backend discovery
// document, translates it to a frontend directory list, then hosts its own
// frontend server to expose the translated data.
type discoveryTranslateService struct {
	serviceConfig

	dir string
}

func (s *discoveryTranslateService) run(c context.Context, f serviceRunFunc) error {
	// Build the Go Managed VM service application.
	p, err := filepath.Abs(s.dir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path [%s]: %s", s.dir, err)
	}

	d, err := ioutil.TempDir(p, "apigen_service")
	if err != nil {
		return err
	}
	defer os.RemoveAll(d)

	svcPath := filepath.Join(d, "service")
	cmd := exec.Command("go", "build", "-o", svcPath, ".")
	cmd.Dir = p
	log.Fields{
		"args": cmd.Args,
		"wd":   cmd.Dir,
	}.Debugf(c, "Executing `go build` command.")
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"dst":        svcPath,
			"wd":         cmd.Dir,
		}.Errorf(c, "Failed to build package:\n%s", string(out))
		return fmt.Errorf("failed to build package: %s", err)
	}

	// Execute the service.
	svc := &killableCommand{
		Cmd: exec.Command(svcPath),
	}
	svc.Env = append(os.Environ(), "LUCI_GO_APPENGINE_APIGEN=1")
	if err := svc.Start(); err != nil {
		return err
	}
	defer svc.kill(c)

	// Load the service's discovery document.
	discoveryURL := url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", s.port),
		Path:   fmt.Sprintf("%s/%s", s.apiRoot, "BackendService.getApiConfigs"),
	}
	data, err := retryHTTP(c, discoveryURL, "POST", "{}")
	if err != nil {
		return fmt.Errorf("endpoints service did not come online: %s", err)
	}

	// Translate the configs into APIs.
	h, err := translateAPIs(data)
	if err != nil {
		return err
	}
	dl, err := h.directoryList()
	if err != nil {
		return fmt.Errorf("failed to create directory list: %s", err)
	}

	// Spawn an HTTP server to host those APIs.
	//
	// We don't care what port it listens on.
	l, err := net.ListenTCP("tcp", &net.TCPAddr{})
	if err != nil {
		return fmt.Errorf("failed to create TCP server: %s", err)
	}
	defer l.Close()

	// Install handlers for directory index/items.
	mux := http.NewServeMux()
	mux.HandleFunc("/", serveJSON(c, dl))
	for _, de := range dl.Items {
		mux.HandleFunc(de.relativeLink("/"), serveJSON(c, de.rdesc))
	}

	server := http.Server{
		Handler: mux,
	}
	go server.Serve(l)

	return f(c, url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", l.Addr().(*net.TCPAddr).Port),
		Path:   "/",
	})
}

// serveJSON returns an HTTP handler that serves the marshalled JSON form of
// the specified object.
func serveJSON(c context.Context, d interface{}) http.HandlerFunc {
	data, err := json.MarshalIndent(d, "", " ")

	return func(w http.ResponseWriter, req *http.Request) {
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
			}.Errorf(c, "Failed to serve.")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		log.Debugf(c, "Serving response data:\n%s", string(data))
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}
}
