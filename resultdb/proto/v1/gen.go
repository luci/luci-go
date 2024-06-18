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

package resultpb

//go:generate cproto
//go:generate svcdec -type ExperimentsServer
//go:generate svcdec -type RecorderServer
//go:generate svcdec -type ResultDBServer
//go:generate mockgen -source resultdb.pb.go -destination resultdb.mock.pb.go -package resultpb -write_package_comment=false
//go:generate mockgen -source recorder.pb.go -destination recorder.mock.pb.go -package resultpb -write_package_comment=false
//go:generate goimports -w resultdb.mock.pb.go recorder.mock.pb.go
