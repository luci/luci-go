// Copyright 2016 The LUCI Authors.
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

// Package gologger is a compatibility layer between go-logging library and
// luci-go/common/logging.
//
// Use it if you want to log to files or console.
//
// Default usage (logging to stderr):
//
//   import (
//     "github.com/luci/luci-go/common/logging"
//     "github.com/luci/luci-go/common/logging/gologger"
//   )
//
//   ...
//
//   ctx := context.Background()
//   ctx = gologger.StdConfig.Use(ctx)
//   logging.Infof(ctx, "Hello %s", "world")
//
//
// If you want more control over where log goes or how it looks, instantiate
// custom LoggerConfig struct:
//
//   logCfg := gologger.LoggerConfig{Out: logFileWriter}
//   ctx = logCfg.Use(ctx)
//
//
// If you know what you are doing you even can prepare go-logging.Logger
// instance yourself and plug it in into luci-go/common/logging:
//
//   logCfg := gologger.LoggerConfig{Logger: goLoggingLogger}
//   ctx = logCfg.Use(ctx)
//
//
// Note that you almost never want to change logging level of the go-logging
// logger (that's why LoggerConfig doesn't expose it), use logging.SetLevel
// instead to do filtering before messages hit go-logging logger.
package gologger
