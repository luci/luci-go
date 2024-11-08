// Copyright 2023 The LUCI Authors.
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

const metricColors = [
  '#4a148c', // Material Purple 900.
  '#0d47a1', // Material Blue 900.
  '#b71c1c', // Material Red 900.
  '#1b5e20', // Material Green 900.
  '#f57f17', // Material Yellow 900.
  '#006064', // Material Cyan 900.
  '#212121', // Material Grey 900.
  '#827717', // Material Lime 900.
  '#880E4F', // Material Pink 900.
  '#bf360c', // Material Deep Orange 900.
];

export const getMetricColor = (metricIndex: number) => {
  return metricColors[metricIndex % metricColors.length];
};
