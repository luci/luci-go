// Copyright 2020 The LUCI Authors.
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

package integrationtests

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/server"

	"go.chromium.org/luci/resultdb/internal/services/backend"
	"go.chromium.org/luci/resultdb/internal/services/purger"
	"go.chromium.org/luci/resultdb/internal/services/recorder"
	"go.chromium.org/luci/resultdb/internal/services/resultdb"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// testApp runs all resultdb servers in one process.
type testApp struct {
	ResultDB pb.ResultDBClient
	Recorder pb.RecorderClient

	servers      []*server.Server
	shutdownOnce sync.Once

	tempDir    string
	authDBPath string
}

func startTestApp(ctx context.Context) (*testApp, error) {
	app, err := newTestApp(ctx)
	if err != nil {
		return nil, err
	}
	if err := app.Start(ctx); err != nil {
		return nil, err
	}
	return app, nil
}

func newTestApp(ctx context.Context) (t *testApp, err error) {
	tempDir, err := ioutil.TempDir("", "resultdb-integration-test")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(tempDir)
		}
	}()

	const authDBTextProto = `
		groups: {
			name: "luci-resultdb-access"
			members: "anonymous:anonymous"
		}
	`
	authDBPath := filepath.Join(tempDir, "authdb.txt")
	if err := ioutil.WriteFile(authDBPath, []byte(authDBTextProto), 0666); err != nil {
		return nil, err
	}

	t = &testApp{
		tempDir:    tempDir,
		authDBPath: authDBPath,
	}
	if err := t.initServers(ctx); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *testApp) Shutdown() {
	t.shutdownOnce.Do(func() {
		var wg sync.WaitGroup
		for _, s := range t.servers {
			s := s
			wg.Add(1)
			go func() {
				defer wg.Done()
				s.Shutdown()
			}()
		}
		wg.Wait()

		if err := os.RemoveAll(t.tempDir); err != nil && !os.IsNotExist(err) {
			log.Printf("failed to remove %q: %s", t.tempDir, err)
		}
	})
}

func (t *testApp) serverClientPair(ctx context.Context, httpPort, adminPort int) (*server.Server, *prpc.Client, error) {
	srvOpts := server.Options{
		AuthDBPath: t.authDBPath,
		HTTPAddr:   fmt.Sprintf("127.0.0.1:%d", httpPort),
		AdminAddr:  fmt.Sprintf("127.0.0.1:%d", adminPort),
		ClientAuth: chromeinfra.DefaultAuthOptions(),
	}
	srv, err := server.New(ctx, srvOpts, nil)
	if err != nil {
		return nil, nil, err
	}

	client := &prpc.Client{
		Host: srvOpts.HTTPAddr,
		Options: &prpc.Options{
			Insecure: true,
		},
	}
	return srv, client, nil
}

func (t *testApp) initServers(ctx context.Context) error {
	// TODO(nodir): use port 0 to let OS choose an available port.
	// This is blocked on server.Server exposing the chosen port.

	// Init resultdb server.
	resultdbServer, resultdbPRPCClient, err := t.serverClientPair(ctx, 8000, 8001)
	if err != nil {
		return err
	}
	err = resultdb.InitServer(resultdbServer, resultdb.Options{
		InsecureSelfURLs: true,
		ContentHostname:  "localhost",
	})
	if err != nil {
		return err
	}

	// Init recorder server.
	recorderServer, recorderPRPCClient, err := t.serverClientPair(ctx, 8010, 8011)
	if err != nil {
		return err
	}
	recorder.InitServer(recorderServer, recorder.Options{
		ExpectedResultsExpiration: time.Hour,
	})

	// Init backend server.
	backendServer, _, err := t.serverClientPair(ctx, 8020, 8021)
	if err != nil {
		return err
	}
	backend.InitServer(backendServer, backend.Options{
		TaskWorkers:         1,
		ForceCronInterval:   100 * time.Millisecond,
		ForceLeaseDuration:  100 * time.Millisecond,
		PurgeExpiredResults: true,
	})

	// Init purger server.
	purgerServer, _, err := t.serverClientPair(ctx, 8030, 8031)
	if err != nil {
		return err
	}
	purger.InitServer(purgerServer, purger.Options{
		ForceCronInterval: 100 * time.Millisecond,
	})

	t.ResultDB = pb.NewResultDBPRPCClient(resultdbPRPCClient)
	t.Recorder = pb.NewRecorderPRPCClient(recorderPRPCClient)
	t.servers = []*server.Server{resultdbServer, recorderServer, backendServer, purgerServer}
	return nil
}

func (t *testApp) ListenAndServe() error {
	eg := errgroup.Group{}
	for _, s := range t.servers {
		s := s
		eg.Go(func() error {
			defer t.Shutdown()
			return s.ListenAndServe()
		})
	}
	return eg.Wait()
}

// Start starts listening and returns when the server is ready to accept
// requests.
func (t *testApp) Start(ctx context.Context) error {
	errC := make(chan error, 1)
	go func() {
		errC <- t.ListenAndServe()
	}()

	// Give servers 5s to start.
	ctx, _ = context.WithTimeout(ctx, 5*time.Second)

outer:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errC:
			if err == nil {
				err = errors.Reason("failed to start").Err()
			}
			return err
		default:
			// OK, see if we are serving.
		}

		for _, s := range t.servers {
			req := &http.Request{
				URL: &url.URL{
					Scheme: "http",
					Host:   s.Options.HTTPAddr,
					Path:   "/healthz",
				},
			}
			req = req.WithContext(ctx)
			switch res, err := http.DefaultClient.Do(req); {
			case err != nil:
				continue outer
			case res.StatusCode != http.StatusOK:
				continue outer
			}
		}

		// All servers are healthy!
		return nil
	}
}
