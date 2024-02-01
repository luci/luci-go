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

import { Info } from '@mui/icons-material';
import { Button } from '@mui/material';
import { observer } from 'mobx-react-lite';

import { useStore } from '@/common/store';
import { DotSpinner } from '@/generic_libs/components/dot_spinner';

const LOADING_VARIANT_INFO_TOOLTIP =
  'It may take several clicks to find any new variant. ' +
  'If you know what your are looking for, please apply a filter instead. ' +
  'This will be improved the in future.';

export const VariantCounts = observer(() => {
  const store = useStore();
  const pageState = store.testHistoryPage;

  const isLoading = pageState.variantsLoader?.isLoading ?? true;
  const canLoadMore = !isLoading && !pageState.variantsLoader?.loadedAll;

  return (
    <>
      Showing <i>{pageState.filteredVariants.length}</i> variant(s) that{' '}
      <i>match(es) the filter</i>.{(canLoadMore || isLoading) && <>&nbsp;</>}
      {(canLoadMore || true) && (
        <>
          <Button
            onClick={() => pageState.variantsLoader?.loadNextPage()}
            endIcon={<Info fontSize="small" />}
            title={LOADING_VARIANT_INFO_TOOLTIP}
          >
            load more
          </Button>
        </>
      )}
      {isLoading && (
        <span className="active-text">
          loading <DotSpinner />
        </span>
      )}
    </>
  );
});
