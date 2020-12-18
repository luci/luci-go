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

package main

// thresholdFile describes format of a threshold JSON file.
// It is used to pick MaxDistance and MaxRank thresholds based on the target
// ChangeRecall.
type thresholdFile struct {
	Thresholds []*threshold `json:"thresholds"`
}

type threshold struct {
	ChangeRecall float64 `json:"changeRecall"`
	MaxDistance  float64 `json:"maxDistance"`
	MaxRank      int     `json:"maxRank"`
}
