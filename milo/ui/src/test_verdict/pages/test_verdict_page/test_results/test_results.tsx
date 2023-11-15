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

import Divider from '@mui/material/Divider';

import { TestResultBundle } from '@/common/services/resultdb';

import { TestResultsContextProvider } from './context';
import { ResultDetails } from './result_details';
import { ResultLogs } from './result_logs';
import { ResultsHeader } from './results_header';

interface Props {
  readonly results: readonly TestResultBundle[];
}

export function TestResults({ results }: Props) {
  return (
    <TestResultsContextProvider results={results}>
      <ResultsHeader />
      <ResultDetails />
      <Divider orientation="horizontal" flexItem />
      <ResultLogs />
    </TestResultsContextProvider>
  );
}
