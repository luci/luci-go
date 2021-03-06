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

// Package spantest implements:
// * start/stop the Cloud Spanner Emulator,
// * creation/removal of a temporary gcloud config,
// * creation of a temporary Spanner instance,
// * creation/destruction of a temporary Spanner database.
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
	"time"

	"cloud.google.com/go/spanner"
	spandb "cloud.google.com/go/spanner/admin/database/apiv1"
	spanins "cloud.google.com/go/spanner/admin/instance/apiv1"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	dbpb "google.golang.org/genproto/googleapis/spanner/admin/database/v1"
	inspb "google.golang.org/genproto/googleapis/spanner/admin/instance/v1"
	"google.golang.org/grpc"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// emulatorCfg is the gcloud config name for Cloud Spanner Emulator.
const emulatorCfg = "spanner-emulator"

// TempDBConfig specifies how to create a temporary database.
type TempDBConfig struct {
	// InstanceName is the name of Spannner instance where to create the
	// temporary database.
	// Format: projects/{project}/instances/{instance}.
	// Defaults to chromeinfra.TestSpannerInstance.
	InstanceName string

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

// NewTempDB creates a temporary database with a random name.
// The caller is responsible for calling Drop on the returned TempDB to
// cleanup resources after usage. Unless it uses Cloud Spanner Emulator, then
// the database will be dropped when emulator stops.
func NewTempDB(ctx context.Context, cfg TempDBConfig, e *Emulator) (*TempDB, error) {
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
	// can be moved in Emulator struct.
	var opts []option.ClientOption
	if e != nil {
		opts = append(opts, e.opts()...)
	} else {
		tokenSource, err := tokenSource(ctx)
		if err != nil {
			return nil, err
		}
		opts = append(opts, option.WithTokenSource(tokenSource))
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

func tokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	opts := chromeinfra.DefaultAuthOptions()
	opts.Scopes = spandb.DefaultAuthScopes()
	a := auth.NewAuthenticator(ctx, auth.SilentLogin, opts)
	if err := a.CheckLoginRequired(); err != nil {
		return nil, errors.Annotate(err, "please login with `luci-auth login -scopes %q`", strings.Join(opts.Scopes, " ")).Err()
	}
	return a.TokenSource()
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

// Emulator is for starting and stopping a Cloud Spanner Emulator process.
// TODO(crbug.com/1066993): Make Emulator an interfact with two implementations (*nativeEmulator and *dockerEmulator).
type Emulator struct {
	// hostport is the address at which emulator process is running.
	hostport string
	// cmd is the command corresponding to in-process emulator, set if running.
	cmd *exec.Cmd
	// cancel cancels the command context to kill the emulator process.
	cancel func()
	// cfgDir is the path to the temporary dircetory holding the gcloud config for emulator.
	cfgDir string
	// containerName is used to explicitly stop the docker container.
	containerName string
}

func (e *Emulator) opts() []option.ClientOption {
	return []option.ClientOption{
		option.WithEndpoint(e.hostport),
		option.WithGRPCDialOption(grpc.WithInsecure()),
		option.WithoutAuthentication(),
	}
}

// createSpannerEmulatorConfig creates a temporary CLOUDSDK_CONFIG, then creates
// a temporary gcloud config for the emulator.
//
// It returns the directory with the configuration. The caller is responsible
// for deleting it when the configuration is no longer needed.
func (e *Emulator) createSpannerEmulatorConfig() (string, error) {
	// TODO(crbug.com/1066993): use os.MkdirTemp after go version is bumped to 1.16.
	tdir, err := ioutil.TempDir("", "gcloud_config")
	if err != nil {
		return "", err
	}
	cmd := exec.Command(
		"/bin/sh",
		"-c",
		fmt.Sprintf("gcloud config configurations create %s;"+
			" gcloud config set auth/disable_credentials true;"+
			" gcloud config set project chops-spanner-testing;"+
			" gcloud config set api_endpoint_overrides/spanner http://localhost:9020/;", emulatorCfg))
	cmd.Env = append(os.Environ(), fmt.Sprintf("CLOUDSDK_CONFIG=%s", tdir))
	return tdir, cmd.Run()
}

// NewInstance creates a temporary instance using Cloud Spanner Emulator.
func (e *Emulator) NewInstance(ctx context.Context, projectName string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if projectName == "" {
		// TODO(crbug.com/1066993): add the default to chromeinfra.
		projectName = "projects/chops-spanner-testing"
	}
	opts := append(e.opts(), option.WithGRPCDialOption(grpc.WithBlock()))

	client, err := spanins.NewInstanceAdminClient(ctx, opts...)
	if err != nil {
		return "", err
	}
	defer client.Close()

	insID := "testing"
	insOp, err := client.CreateInstance(ctx, &inspb.CreateInstanceRequest{
		Parent:     projectName,
		InstanceId: insID,
		Instance: &inspb.Instance{
			Config:      emulatorCfg,
			DisplayName: insID,
			NodeCount:   1,
		},
	})
	if err != nil {
		return "", errors.Annotate(err, "failed to create instance").Err()
	}

	switch ins, err := insOp.Wait(ctx); {
	case err != nil:
		return "", errors.Annotate(err, "failed to get instance state").Err()
	case ins.State != inspb.Instance_READY:
		return "", fmt.Errorf("instance is not ready, got state %v", ins.State)
	default:
		return ins.Name, nil
	}
}
