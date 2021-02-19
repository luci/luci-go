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
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
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
	"go.chromium.org/luci/common/system/port"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// emulatorRelativePath is the relative path of the Cloud Spanner Emulator binary to gcloud root.
const emulatorRelativePath = "bin/cloud_spanner_emulator/emulator_main"

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
	client, err := spandb.NewDatabaseAdminClient(ctx, db.opts...)
	if err != nil {
		return err
	}
	defer client.Close()

	return client.DropDatabase(ctx, &dbpb.DropDatabaseRequest{
		Database: db.Name,
	})
}

var dbNameAlphabetInversedRe = regexp.MustCompile(`[^\w]+`)

// NewTempDB creates a temporary database using Cloud Spanner Emulator.
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
	// TODO(crbug.com/1066993): require Spanner emulator, then NewTempDB function
	// can be moved un Emulator struct.
	var opts []option.ClientOption
	if emulatorAddr := os.Getenv("SPANNER_EMULATOR_HOST"); emulatorAddr != "" {
		opts = append(
			opts,
			emulatorOptions(emulatorAddr)...,
		)
	} else {
		creds, err := cfg.credentials(ctx)
		if err != nil {
			return nil, err
		}
		opts = append(opts, option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)))
	}

	client, err := spandb.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	// Generate a random database name.
	var random uint32
	if err := binary.Read(rand.Reader, binary.LittleEndian, &random); err != nil {
		panic(err)
	}
	dbName := fmt.Sprintf("tmp%s-%d", time.Now().Format("20060102-"), random)
	dbName = SanitizeDBName(dbName)

	dbOp, err := client.CreateDatabase(ctx, &dbpb.CreateDatabaseRequest{
		Parent:          instanceName,
		CreateStatement: "CREATE DATABASE " + dbName,
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

// SanitizeDBName tranforms name to a valid one.
// If name is already valid, returns it without changes.
func SanitizeDBName(name string) string {
	name = strings.ToLower(name)
	name = dbNameAlphabetInversedRe.ReplaceAllLiteralString(name, "_")
	const maxLen = 30
	if len(name) > maxLen {
		name = name[:maxLen]
	}
	name = strings.TrimRight(name, "_")
	return name
}

func emulatorOptions(emulatorAddr string) []option.ClientOption {
	return []option.ClientOption{
		option.WithEndpoint(emulatorAddr),
		option.WithGRPCDialOption(grpc.WithInsecure()),
		option.WithoutAuthentication(),
	}
}

func findEmulatorPath() (string, error) {
	o, err := exec.Command("gcloud", "info", "--format=value(installation.sdk_root)").Output()
	if err != nil {
		return "", err
	}

	emulatorPath := fmt.Sprintf("%s/%s", strings.TrimSuffix(string(o), "\n"), emulatorRelativePath)
	_, err = os.Stat(emulatorPath)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("cannot find cloud spanner emulator binary at %v", emulatorPath)
	}
	return emulatorPath, nil
}

// Emulator is for starting and stopping a Cloud Spanner Emulator process.
type Emulator struct {
	// hostport is the address at which emulator process is running.
	hostport string
	// cmd is the command corresponding to in-process emulator, set if running.
	cmd *exec.Cmd
	// once is for Stop that should cleanup only once.
	once sync.Once
}

// StartSpannerEmulator starts a Cloud Spanner Emulator instance.
func StartSpannerEmulator(ctx context.Context) (*Emulator, error) {
	errorAnnotation := "start emulator"
	emulatorPath, err := findEmulatorPath()
	if err != nil {
		return nil, errors.Annotate(err, errorAnnotation).Err()
	}

	p, err := port.PickUnusedPort()
	if err != nil {
		return nil, errors.Annotate(err, errorAnnotation).Err()
	}
	emulator := &Emulator{
		hostport: fmt.Sprintf("localhost:%d", p),
	}

	emulator.cmd = exec.Command(emulatorPath, "--host_port", emulator.hostport)
	emulator.cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	emulator.cmd.Stdout = os.Stdout
	emulator.cmd.Stderr = os.Stderr

	return emulator, emulator.cmd.Start()
}

func (emulator *Emulator) Stop() {
	emulator.once.Do(func() {
		if emulator.cmd != nil {
			emulator.cmd.Process.Release()
			emulator.cmd.Process.Kill()
			emulator.cmd = nil
		}
	})
}

// CreateSpannerEmulatorConfig creates a temporary CLOUDSDK_CONFIG, then creates
// a temporary gcloud config for the emulator.
//
// It returns the directory with the configuration. The caller is responsible
// for deleting it when the configuration is no longer needed.
func (emulator *Emulator) CreateSpannerEmulatorConfig() (string, error) {
	// TODO(crbug.com/1066993): use os.MkdirTemp after go version is bumped to 1.16.
	tdir, err := ioutil.TempDir("", "gcloud_config")
	if err != nil {
		return "", err
	}

	os.Setenv("CLOUDSDK_CONFIG", tdir)
	// TODO(crbug.com/1066993): use https://pkg.go.dev/os/exec#Cmd.Env instead
	// after emulator is required.
	os.Setenv("SPANNER_EMULATOR_HOST", emulator.hostport)
	cmd := exec.Command(
		"/bin/sh",
		"-c",
		fmt.Sprintf("gcloud config configurations create spanner-emulator;"+
			" gcloud config set auth/disable_credentials true;"+
			" gcloud config set project chops-spanner-testing;"+
			" gcloud config set api_endpoint_overrides/spanner http://localhost:9020/;"))
	return tdir, cmd.Run()
}

// NewTempInstance creates a temporary instance using Cloud Spanner Emulator.
func (emulator *Emulator) NewTempInstance(ctx context.Context, projectName string) (string, error) {
	timeout := 30 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if projectName == "" {
		// TODO(crbug.com/1066993): add the default to chromeinfra.
		projectName = "projects/chops-spanner-testing"
	}
	errAnnotation := "failed to create instance"

	opts := emulatorOptions(emulator.hostport)
	opts = append(opts, option.WithGRPCDialOption(grpc.WithBlock()))

	client, err := spanins.NewInstanceAdminClient(ctx, opts...)
	if err != nil {
		return "", err
	}
	defer client.Close()

	insId := "testing"
	insOp, err := client.CreateInstance(ctx, &inspb.CreateInstanceRequest{
		Parent:     projectName,
		InstanceId: insId,
		Instance: &inspb.Instance{
			Config:      "spanner-emulator",
			DisplayName: insId,
			NodeCount:   1,
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
