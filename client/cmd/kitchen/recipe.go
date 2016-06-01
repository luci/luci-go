// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/client/cmd/kitchen/proto"
)

// readConfig reads and parses recipe config at cfgPath.
func readConfig(cfgPath string) (*recipe_engine.Package, error) {
	contents, err := ioutil.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read %q: %s", cfgPath, err)
	}

	var pkg recipe_engine.Package
	if err := proto.UnmarshalText(string(contents), &pkg); err != nil {
		return nil, fmt.Errorf("cannot parse %q: %s", cfgPath, err)
	}
	return &pkg, nil
}

// RecipeRun is parameters for running a recipe.
type recipeRun struct {
	repositoryPath string

	// The following are command line recipes.py command line arguments.

	recipe               string
	propertiesJSON       string
	propertiesFile       string
	outputResultJSONFile string
	workDir              string // Where to run the recipe.
}

// Command creates a exec.Cmd for running a recipe.
func (r *recipeRun) Command() (*exec.Cmd, error) {
	cfgPath := filepath.Join(r.repositoryPath, "infra/config/recipes.cfg")
	cfg, err := readConfig(cfgPath)
	if err != nil {
		return nil, err
	}

	if cfg.RecipesPath == nil || *cfg.RecipesPath == "" {
		return nil, fmt.Errorf("recipe_path is unspecified in %q", cfgPath)
	}

	recipesPy := path.Join(r.repositoryPath, *cfg.RecipesPath, "recipes.py")
	if _, err := os.Stat(recipesPy); os.IsNotExist(err) {
		return nil, fmt.Errorf("%q does not exist", recipesPy)
	}

	cmd := exec.Command(
		"python", recipesPy,
		"run",
		"--properties", r.propertiesJSON,
		"--properties-file", r.propertiesFile,
		"--workdir", r.workDir,
		"--output-result-json", r.outputResultJSONFile,
		r.recipe,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, nil
}
