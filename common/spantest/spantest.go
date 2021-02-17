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

// Package spantest implements creation/destruction of a temporary Spanner
// database.
package spantest

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	mrand "math/rand"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	spandb "cloud.google.com/go/spanner/admin/database/apiv1"
	spanins "cloud.google.com/go/spanner/admin/instance/apiv1"
	"google.golang.org/api/option"
	dbpb "google.golang.org/genproto/googleapis/spanner/admin/database/v1"
	inspb "google.golang.org/genproto/googleapis/spanner/admin/instance/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// TempDBConfig specifies how to create a temporary database.
type TempDBConfig struct {
	// InstanceName is the name of Spannner instance where to create the
	// temporary database.
	// Format: projects/{project}/instances/{instance}.
	// Defaults to chromeinfra.TestSpannerInstance.
	InstanceName string

	// Credentials will be used to authenticate to Spanner.
	// If nil, auth.Authenticator with SilentLogin and chrome-infra auth options
	// will be used.
	// This means that that the user may have to login with luci-auth tool.
	Credentials credentials.PerRPCCredentials

	// InitScriptPath is a path to a DDL script to initialize the database.
	//
	// In lieu of a proper DDL parser, it is parsed using regexes.
	// Therefore the script MUST:
	//   - Use `#`` and/or `--`` for comments. No block comments.
	//   - Separate DDL statements with `;\n`.
	//
	// If empty, the database is created with no tables.
	InitScriptPath string
}

func (cfg *TempDBConfig) credentials(ctx context.Context) (credentials.PerRPCCredentials, error) {
	if cfg.Credentials != nil {
		return cfg.Credentials, nil
	}

	opts := chromeinfra.DefaultAuthOptions()
	opts.Scopes = spandb.DefaultAuthScopes()
	a := auth.NewAuthenticator(ctx, auth.SilentLogin, opts)
	if err := a.CheckLoginRequired(); err != nil {
		return nil, errors.Annotate(err, "please login with `luci-auth login -scopes %q`", strings.Join(opts.Scopes, " ")).Err()
	}
	return a.PerRPCCredentials()
}

var ddlStatementSepRe = regexp.MustCompile(`;\s*\n`)
var commentRe = regexp.MustCompile(`(--|#)[^\n]*`)

// readDDLStatements read the file at cfg.InitScriptPath as a sequence of DDL
// statements. If the path is empty, returns (nil, nil).
func (cfg *TempDBConfig) readDDLStatements() ([]string, error) {
	if cfg.InitScriptPath == "" {
		return nil, nil
	}

	contents, err := ioutil.ReadFile(cfg.InitScriptPath)
	if err != nil {
		return nil, err
	}

	statements := ddlStatementSepRe.Split(string(contents), -1)
	ret := statements[:0]
	for _, stmt := range statements {
		stmt = commentRe.ReplaceAllString(stmt, "")
		stmt = strings.TrimSpace(stmt)
		if stmt != "" {
			ret = append(ret, stmt)
		}
	}
	return ret, nil
}

// adminClient returns a Spanner admin client, it must be closed when done.
func adminClient(ctx context.Context, opts []option.ClientOption) (*spandb.DatabaseAdminClient, error) {
	return spandb.NewDatabaseAdminClient(ctx, opts...)
}

// TempDB is a temporary Spanner database.
type TempDB struct {
	Name string
	opts []option.ClientOption
}

// Client returns a spanner client connected to the database.
func (db *TempDB) Client(ctx context.Context) (*spanner.Client, error) {
	return spanner.NewClient(ctx, db.Name, db.opts...)
}

// Drop deletes the database.
func (db *TempDB) Drop(ctx context.Context) error {
	client, err := adminClient(ctx, db.opts)
	if err != nil {
		return err
	}
	defer client.Close()

	return client.DropDatabase(ctx, &dbpb.DropDatabaseRequest{
		Database: db.Name,
	})
}

var nameAlphabetInversedRe = regexp.MustCompile(`[^\w]+`)

// NewTempDB creates a temporary database with a random name.
// The caller is responsible for calling Drop on the returned TempDB to
// cleanup resources after usage.
func NewTempDB(ctx context.Context, cfg TempDBConfig) (*TempDB, error) {
	instanceName := cfg.InstanceName
	if instanceName == "" {
		instanceName = chromeinfra.TestSpannerInstance
	}

	initStatements, err := cfg.readDDLStatements()
	if err != nil {
		return nil, errors.Annotate(err, "failed to read %q", cfg.InitScriptPath).Err()
	}

	// Use Spanner emulator if available.
	// TODO(crbug.com/1066993): require Spanner emulator.
	var opts []option.ClientOption
	if emulatorAddr := os.Getenv("SPANNER_EMULATOR_HOST"); emulatorAddr != "" {
		opts = append(
			opts,
			option.WithEndpoint(emulatorAddr),
			option.WithGRPCDialOption(grpc.WithInsecure()),
			option.WithoutAuthentication(),
		)
	} else {
		creds, err := cfg.credentials(ctx)
		if err != nil {
			return nil, err
		}
		opts = append(opts, option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)))
	}

	client, err := adminClient(ctx, opts)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	dbOp, err := client.CreateDatabase(ctx, &dbpb.CreateDatabaseRequest{
		Parent:          instanceName,
		CreateStatement: "CREATE DATABASE " + "tempdb",
		ExtraStatements: initStatements,
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to create database").Err()
	}
	db, err := dbOp.Wait(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create database").Err()
	}

	return &TempDB{
		Name: db.Name,
		opts: opts,
	}, nil
}

// NewTempInstance creates a temporary instance with a random name.
func NewTempInstance(ctx context.Context, projectName string) (string, error) {
	if projectName == "" {
		// TODO(crbug.com/1066993): add the default to chromeinfra.
		projectName = "projects/chops-spanner-testing"
	}
	errAnnotation := "failed to create instance"
	emulatorAddr := os.Getenv("SPANNER_EMULATOR_HOST")
	if emulatorAddr == "" {
		return "", errors.Annotate(fmt.Errorf("cloud spanner emulator address not found"), errAnnotation).Err()
	}

	opts := []option.ClientOption{
		option.WithEndpoint(emulatorAddr),
		option.WithGRPCDialOption(grpc.WithInsecure()),
		option.WithoutAuthentication(),
	}
	client, err := spanins.NewInstanceAdminClient(ctx, opts...)
	if err != nil {
		return "", err
	}
	defer client.Close()

	// Generate a random database name.
	insId := "testing"
	insOp, err := client.CreateInstance(ctx, &inspb.CreateInstanceRequest{
		Parent:     projectName,
		InstanceId: insId,
		Instance: &inspb.Instance{
			Config:      "spanner-emulator",
			DisplayName: insId,
			NodeCount:   int32(1),
		},
	})
	if err != nil {
		return "", errors.Annotate(err, errAnnotation).Err()
	}

	ins, err := insOp.Wait(ctx)
	if err != nil {
		return "", errors.Annotate(err, errAnnotation).Err()
	}
	if ins.State != inspb.Instance_READY {
		return "", errors.Annotate(fmt.Errorf("instance is not ready, got state %v", ins.State), errAnnotation).Err()
	}

	return ins.Name, nil
}

// stopSpannerEmulator stops Cloud Spanner Emulator when all temporary gcloud
// config has been removed.
func stopSpannerEmulator() {
	files, err := ioutil.ReadDir(os.TempDir())
	if err != nil {
		fmt.Println(err)
	}

	for _, f := range files {
		if strings.HasPrefix(f.Name(), "gcloud_config") {
			fmt.Println("not ready to stop emulator yet")
			return
		}
	}

	fmt.Println("Stopping Cloud Spanner Emulator")
	exec.Command("pkill", "-f", "cloud_spanner_emulator").Run()
}

type Emulator struct {
	Host     string
	RestPort int
	Stop     func()
}

// StartSpannerEmulator starts Cloud Spanner Emulator if there's not one already
// running.
func StartSpannerEmulator(ctx context.Context) (*Emulator, error) {
	mrand.Seed(time.Now().UnixNano())
	emu := &Emulator{
		Host:     fmt.Sprintf("localhost:%d", mrand.Intn(100)+9000),
		RestPort: mrand.Intn(100) + 9000,
	}

	fmt.Println(fmt.Sprintf("Starting Cloud Spanner Emulator at host: %s, restport: %d", emu.Host, emu.RestPort))
	cmd := exec.Command("gcloud", "beta", "emulators", "spanner", "start", "--host-port", emu.Host, "--rest-port", fmt.Sprintf("%d", emu.RestPort))

	emu.Stop = func() {
		stopSpannerEmulator()
	}

	out, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	c := make(chan error, 0)
	go func() {
		defer close(c)
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "Cloud Spanner emulator running.") {
				return
			}
		}
		c <- scanner.Err()
	}()

	if err = cmd.Start(); err != nil {
		return nil, err
	}

	select {
	case err = <-c:
	case <-ctx.Done():
		err = fmt.Errorf("deadline exceeded")
	}
	return emu, err
}

// CreateSpannerEmulatorConfig creates a temporary CLOUDSDK_CONFIG then creates
// a temporary gcloud config for the emulator.
//
// It returns the temporary directory for later clean up.
func (emu *Emulator) CreateSpannerEmulatorConfig() (string, error) {
	tdir, err := ioutil.TempDir("", "gcloud_config")
	if err != nil {
		return "", err
	}

	os.Setenv("CLOUDSDK_CONFIG", tdir)
	os.Setenv("SPANNER_EMULATOR_HOST", emu.Host)
	cmd := exec.Command(
		"/bin/sh",
		"-c",
		fmt.Sprintf("gcloud config configurations create spanner-emulator;"+
			" gcloud config set auth/disable_credentials true;"+
			" gcloud config set project chops-spanner-testing;"+
			" gcloud config set api_endpoint_overrides/spanner http://localhost:%d/;", emu.RestPort))
	return tdir, cmd.Run()
}
