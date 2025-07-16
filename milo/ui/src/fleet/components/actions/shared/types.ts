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

// This file contains shared types for fleet actions.

// Simplified type containing the minimal DUT info required for running an
// autorepair job.
export interface DutToRepair {
  // Same as dut_name label. dut_name is used for 1.) the main identifier for
  // which DUT we are referring to and 2.) populating tags shown in UIs like
  // LUCI or read by downstream services.
  name: string;
  // DUT ID is used to schedule a task against a particular DUT in Swarming.
  // DUT ID is also often referred to as "asset tag".
  dutId: string;
  state?: string;

  // Optional extra data fields used for filing repair bugs.
  board?: string;
  model?: string;

  // Note that this is label-pool, not Swarming pool.
  pool?: string;
}
