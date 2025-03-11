// Copyright 2025 The LUCI Authors.
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

// Package sqldb is a package that puts a *sql.DB into the context of every request and retrieves it for you.  It also
// initializes and deinitializes the database connection.
//
// This package is an outgrowth of the fleet console service https://pkg.go.dev/go.chromium.org/infra/fleetconsole,
// which takes inspiration (nearly verbatim inspiration) from device manager
// https://pkg.go.dev/go.chromium.org/infra/device_manager.
//
// Its goal is to make it easier to connect to a SQL database from a LUCI server application without a lot of
// boilerplate code, and to correctly handle getting the database credential from secret manager (accessed via the LUCI
// secrets library).
//
// https://pkg.go.dev/go.chromium.org/luci/server/secrets
//
// It understands the following flags:
//
//  -sqldb-password-secret
//  -sqldb-connection-url
//
// Note that the driver is implied by the scheme of the URL. This is to allow you to talk to
// "postgres://username@localhost:5000/dbname" using "pgx://username@localhost:5000/dbname".
//
// Also, note that the exact details of how the driver and the connection URL interact is easily the most contentious
// part of the design of this library. Another way of thinking about it is that this is where we have chosen to spend
// our complexity budget. The rest of the module is pretty straightforward.
//
// This package also does not support multiple, simultaneous DB connections. We thought about it, and we decided not to
// do it. There is a path forward, though, if we need to add this in the future without breaking backwards
// compatibility.  That is to have the concept of a "main" database, specified in some way which will use the current
// API, but also some additional functions for accessing a DB connection by a string name.
//
package sqldb
