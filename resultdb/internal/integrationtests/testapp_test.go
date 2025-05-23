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
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/server"

	"go.chromium.org/luci/resultdb/internal/services/deadlineenforcer"
	"go.chromium.org/luci/resultdb/internal/services/finalizer"
	"go.chromium.org/luci/resultdb/internal/services/purger"
	"go.chromium.org/luci/resultdb/internal/services/recorder"
	"go.chromium.org/luci/resultdb/internal/services/resultdb"
	"go.chromium.org/luci/resultdb/internal/services/testmetadataupdator"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// testApp runs all resultdb servers in one process.
type testApp struct {
	ResultDB pb.ResultDBClient
	Recorder pb.RecorderClient

	servers      []*server.Server
	shutdownOnce sync.Once

	t          testing.TB
	tempDir    string
	authDBPath string
}

func startTestApp(ctx context.Context, t testing.TB) (*testApp, error) {
	app, err := newTestApp(ctx, t)
	if err != nil {
		return nil, err
	}
	if err := app.Start(ctx); err != nil {
		return nil, err
	}
	return app, nil
}

func newTestApp(ctx context.Context, t testing.TB) (ta *testApp, err error) {
	tempDir := t.TempDir()

	const authDBTextProto = `
		groups: {
			name: "luci-resultdb-access"
			members: "anonymous:anonymous"
		}
		realms: {
			api_version: 1
			permissions: {
				name: "resultdb.invocations.create"
			}
			permissions: {
				name: "resultdb.invocations.get"
			}
			permissions: {
				name: "resultdb.invocations.include"
			}
			realms: {
				name: "testproject:testrealm"
				bindings: {
					permissions: 0
					permissions: 1
					permissions: 2
					principals: "anonymous:anonymous"
				}
			}
		}
	`
	authDBPath := filepath.Join(tempDir, "authdb.txt")
	if err := os.WriteFile(authDBPath, []byte(authDBTextProto), 0666); err != nil {
		return nil, err
	}

	ta = &testApp{
		t:          t,
		tempDir:    tempDir,
		authDBPath: authDBPath,
	}
	if err := ta.initServers(ctx); err != nil {
		return nil, err
	}
	t.Cleanup(ta.shutdown)
	return ta, nil
}

func (t *testApp) shutdown() {
	t.shutdownOnce.Do(func() {
		var wg sync.WaitGroup
		for _, s := range t.servers {
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
		ArtifactRBEInstance: "projects/luci-resultdb-dev/instances/artifacts",
		InsecureSelfURLs:    true,
		ContentHostnameMap:  map[string]string{"*": "localhost"},
	})
	if err != nil {
		return err
	}

	// Init recorder server.
	recorderServer, recorderPRPCClient, err := t.serverClientPair(ctx, 8010, 8011)
	if err != nil {
		return err
	}
	err = recorder.InitServer(recorderServer, recorder.Options{
		ArtifactRBEInstance:       "projects/luci-resultdb-dev/instances/artifacts",
		ExpectedResultsExpiration: time.Hour,
	})
	if err != nil {
		return err
	}

	// Init finalizer server.
	finalizerServer, _, err := t.serverClientPair(ctx, 8020, 8021)
	if err != nil {
		return err
	}
	opts := finalizer.Options{
		ResultDBHostname: "rdb-host",
	}
	finalizer.InitServer(finalizerServer, opts)

	// bqexporter is not needed.

	// Init purger server.
	purgerServer, _, err := t.serverClientPair(ctx, 8030, 8031)
	if err != nil {
		return err
	}
	purger.InitServer(purgerServer, purger.Options{
		ForceCronInterval: 100 * time.Millisecond,
	})

	// Init deadlineenforcer server.
	deadlineEnforcerServer, _, err := t.serverClientPair(ctx, 8040, 8041)
	if err != nil {
		return err
	}
	deadlineenforcer.InitServer(deadlineEnforcerServer, deadlineenforcer.Options{
		ForceCronInterval: 100 * time.Millisecond,
	})

	// Init update testmetadataupdator server.
	testMetadataUpdatorServer, _, err := t.serverClientPair(ctx, 8050, 8051)
	if err != nil {
		return err
	}
	testmetadataupdator.InitServer(testMetadataUpdatorServer)

	t.ResultDB = pb.NewResultDBPRPCClient(resultdbPRPCClient)
	t.Recorder = pb.NewRecorderPRPCClient(recorderPRPCClient)
	t.servers = []*server.Server{resultdbServer, recorderServer, finalizerServer, purgerServer, deadlineEnforcerServer, testMetadataUpdatorServer}
	return nil
}

func (t *testApp) Serve() error {
	eg := errgroup.Group{}
	for _, s := range t.servers {
		eg.Go(func() error {
			defer t.shutdown()
			return s.Serve()
		})
	}
	return eg.Wait()
}

// Start starts listening and returns when the server is ready to accept
// requests.
func (t *testApp) Start(ctx context.Context) error {
	errC := make(chan error, 1)
	go func() {
		errC <- t.Serve()
	}()

	// Give servers 5s to start.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

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
