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

import { LinearProgress } from '@mui/material';
import { observer } from 'mobx-react-lite';
import { useEffect } from 'react';
import { useParams, useSearchParams } from 'react-router-dom';

import { useStore } from '../../store';
import { GraphType } from '../../store/test_history_page';
import { DateAxis } from './date_axis';
import { DurationGraph } from './duration_graph';
import { DurationLegend } from './duration_legend';
import { FilterBox } from './filter_box';
import { GraphConfig } from './graph_config';
import { StatusGraph } from './status_graph';
import { TestIdLabel } from './test_id_label';
import { VariantCounts } from './variant_counts';
import { VariantDefTable } from './variant_def_table';
import { VerdictDetailsDialog } from './verdict_details_dialog';

/**
 * Maps graph type to the corresponding component.
 */
const GRAPH_TYPE_COMPONENT_MAP = {
  [GraphType.DURATION]: DurationGraph,
  [GraphType.STATUS]: StatusGraph,
};

export const TestHistoryPage = observer(() => {
  const { realm, testId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const initialFilterText = searchParams.get('q');

  const store = useStore();
  const pageState = store.testHistoryPage;

  if (!realm || !testId) {
    throw new Error('invariant violated: realm and testId should be set');
  }

  useEffect(() => {
    pageState.setParams(realm, testId);
  }, [pageState, realm, testId]);

  useEffect(
    () => {
      if (!initialFilterText) {
        return;
      }
      pageState.setFilterText(initialFilterText);
    },
    // Do not declare initialFilterText as a dep because we only want to set it
    // when the page is being intialized.
    [pageState]
  );

  // Update the querystring when filters are updated.
  useEffect(() => {
    setSearchParams({ ...(!pageState.filterText ? {} : { q: pageState.filterText }) }, { replace: true });
  }, [pageState.filterText]);

  useEffect(() => {
    pageState.entriesLoader?.loadFirstPage();
  }, [pageState.entriesLoader]);

  useEffect(() => {
    pageState.variantsLoader?.loadFirstPage();
  }, [pageState.variantsLoader]);

  const Graph = GRAPH_TYPE_COMPONENT_MAP[pageState.graphType];

  return (
    <div
      css={{
        display: 'grid',
        width: '100%',
        minWidth: '800px',
        gridTemplateRows: 'auto auto 1fr',
      }}
    >
      <TestIdLabel realm={realm} testId={testId} />
      <LinearProgress value={100} variant="determinate" />
      <FilterBox css={{ width: 'calc(100% - 10px)', margin: '5px' }} />
      <GraphConfig />
      <div
        css={{
          display: 'grid',
          margin: '0 5px',
          gridTemplateColumns: 'auto 1fr auto',
          gridTemplateRows: 'auto 1fr',
          gridTemplateAreas: `
          'v-table x-axis extra'
          'v-table graph extra'`,
        }}
      >
        <VariantDefTable css={{ gridArea: 'v-table' }} />
        <DateAxis css={{ gridArea: 'x-axis' }} />
        <Graph css={{ gridArea: 'graph' }} />
        {pageState.graphType === GraphType.DURATION && <DurationLegend css={{ gridArea: 'extra' }} />}
      </div>
      <div css={{ padding: '5px' }}>
        <VariantCounts />
      </div>
      <VerdictDetailsDialog />
      {/* Add padding to support free scrolling when the dialog is open. */}
      {!pageState.selectedGroup && <div css={{ width: '100%', height: '60vh' }} />}
    </div>
  );
});
