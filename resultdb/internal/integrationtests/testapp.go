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
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/server"

	"go.chromium.org/luci/resultdb/internal/backend"
	"go.chromium.org/luci/resultdb/internal/recorder"
	"go.chromium.org/luci/resultdb/internal/resultdb"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// testApp runs all resultdb servers in one process.
type testApp struct {
	servers  []*server.Server
	resultdb pb.ResultDBClient
	recorder pb.RecorderClient

	shutdownOnce sync.Once

	tempDir    string
	authDBPath string
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
	if err := ioutil.WriteFile(authDBPath, []byte(authDBTextProto), 0777); err != nil {
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
		for _, s := range t.servers {
			s.Shutdown()
		}
		if err := os.RemoveAll(t.tempDir); err != nil && !os.IsNotExist(err) {
			log.Printf("failed to remove %q: %s", t.tempDir, err)
		}
	})
}

func (t *testApp) serverClientPair(ctx context.Context, httpPort, adminPort int) (*server.Server, *prpc.Client, error) {
	srvOpts := server.Options{
		AuthDBPath:               t.authDBPath,
		HTTPAddr:                 fmt.Sprintf("127.0.0.1:%d", httpPort),
		AdminAddr:                fmt.Sprintf("127.0.0.1:%d", adminPort),
		ClientAuth:               chromeinfra.DefaultAuthOptions(),
		LimiterMaxConcurrentRPCs: 10,
	}
	srv, err := server.New(ctx, srvOpts, nil)
	if err != nil {
		return nil, nil, err
	}

	client := &prpc.Client{
		Host: fmt.Sprintf("localhost:%d", httpPort),
		Options: &prpc.Options{
			Insecure: true,
		},
	}
	return srv, client, nil
}

func (t *testApp) initServers(ctx context.Context) (err error) {
	// Init resultdb server.
	resultdbServer, resultdbPRPCClient, err := t.serverClientPair(ctx, 8000, 8001)
	if err != nil {
		return err
	}
	resultdb.InitServer(resultdbServer, resultdb.Options{
		InsecureSelfURLs: true,
		ContentHostname:  "localhost",
	})

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
	backend.InitServer(recorderServer, backend.Options{
		PurgeExiredResults: true,
	})

	t.resultdb = pb.NewResultDBPRPCClient(resultdbPRPCClient)
	t.recorder = pb.NewRecorderPRPCClient(recorderPRPCClient)
	t.servers = []*server.Server{resultdbServer, recorderServer, backendServer}
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
