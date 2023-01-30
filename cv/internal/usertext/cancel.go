// Copyright 2021 The LUCI Authors.
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

package usertext

// StoppedRun is the default message put in the reason of the attention set
// change when CV stops a run.
//
// Ideally, modules set the message with the reason of the stop so that
// users are given more detailed info for the reason of the attention set
// changes that they received a notification for.
//
// TODO(crbug/1251469) - find if this is still needed, once all the places
// are update to specify the reason of cancellation.
const StoppedRun = "CV stopped processing the Run"
