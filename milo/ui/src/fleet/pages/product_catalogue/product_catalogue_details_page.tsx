// Copyright 2026 The LUCI Authors.
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

import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import { Box, IconButton, Typography, Alert, TextField } from '@mui/material';
import {
  MaterialReactTable,
  MRT_ColumnDef,
  MRT_Row,
} from 'material-react-table';
import { useEffect, useMemo, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import AlertWithFeedback from '@/fleet/components/feedback/alert_with_feedback';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { getFeatureFlag } from '@/fleet/config/features';
import {
  CATALOG_PATH,
  generateCatalogDetailsURL,
} from '@/fleet/constants/paths';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { getErrorMessage } from '@/fleet/utils/errors';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { ProductCatalogEntry } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { OrderForm } from './order_form';
import { COLUMNS } from './product_catalogue_columns';
import { useProductCatalogDetailsData } from './use_product_catalog_details_data';

interface DetailRow {
  readonly key: string;
  readonly rawValue: unknown;
}

interface DetailValueCellProps {
  readonly row: MRT_Row<DetailRow>;
  readonly entry: ProductCatalogEntry | undefined;
}

const DetailValueCell = ({ row, entry }: DetailValueCellProps) => {
  const fieldKey = row.original.key;
  const rawValue = row.original.rawValue;

  const col = COLUMNS.find((c) => c.header === fieldKey);
  if (!col) return <>{String(rawValue ?? '')}</>;

  const accessor = col.accessorKey as keyof ProductCatalogEntry;

  if (accessor === 'productCatalogId') {
    return <>{String(rawValue ?? '')}</>;
  }

  if (col.Cell && entry) {
    // SAFETY: We cast the column's Cell renderer to a component accepting our mocked
    // props. This is safe because our cell renderers in COLUMNS only rely on
    // cell.getValue() and row.original, which we mock here with compatible shapes.
    const CellRenderer = col.Cell as unknown as React.ComponentType<{
      cell: { getValue: () => unknown };
      row: { original: ProductCatalogEntry };
      column: typeof col;
      table: Record<string, never>;
    }>;

    return (
      <CellRenderer
        cell={{
          getValue: () => rawValue,
        }}
        row={{
          original: entry,
        }}
        column={col}
        table={{}}
      />
    );
  }

  if (Array.isArray(rawValue)) {
    return <>{rawValue.join(', ')}</>;
  }

  return <>{String(rawValue ?? '')}</>;
};

const useNavigatedFromLink = () => {
  const { state } = useLocation();

  const [navigatedFromLink, setNavigatedFromLink] = useState<
    string | undefined
  >(undefined);

  // state from useLocation is lost on rerenders so we need to save it in a react state
  useEffect(() => {
    if (state) {
      setNavigatedFromLink(state.navigatedFromLink);
    }
  }, [state]);

  return navigatedFromLink;
};

export const ProductCatalogDetailsPage = () => {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const navigatedFromLink = useNavigatedFromLink();
  const requestFilingEnabled = getFeatureFlag('RequestFiling');

  const navigateToCatalogIfChanged = (newId: string) => {
    if (id !== newId) {
      navigate(generateCatalogDetailsURL(newId), {
        state: { navigatedFromLink },
      });
    }
  };

  const { error, isError, isLoading, entry } = useProductCatalogDetailsData(id);

  const columns = useMemo<MRT_ColumnDef<DetailRow>[]>(
    () => [
      {
        accessorKey: 'key',
        header: 'Field',
        size: 200,
        muiTableBodyCellProps: {
          sx: {
            fontWeight: 500,
            color: 'text.secondary',
          },
        },
      },
      {
        accessorKey: 'rawValue',
        header: 'Value',
        size: 600,
        Cell: ({ row }: { readonly row: MRT_Row<DetailRow> }) => (
          <DetailValueCell row={row} entry={entry} />
        ),
      },
    ],
    [entry],
  );

  const data = useMemo<DetailRow[]>(() => {
    if (!entry) return [];
    return COLUMNS.map((col) => {
      const accessor = col.accessorKey as keyof ProductCatalogEntry;
      return {
        key: col.header as string,
        rawValue: entry[accessor],
      };
    });
  }, [entry]);

  const table = useFCDataTable({
    columns,
    data,
    enableRowVirtualization: false,
    enablePagination: false,
    enableColumnActions: false,
    enableSorting: false,
    enableTopToolbar: false,
    enableStickyHeader: true,
    muiTableContainerProps: {
      sx: {
        maxWidth: '100%',
        overflowX: 'hidden',
        margin: '24px 0',
      },
    },
  });

  if (isError) {
    return (
      <Box sx={{ p: 3 }}>
        <Alert severity="error">
          Something went wrong:{' '}
          {getErrorMessage(error, 'fetch product catalog entry')}
        </Alert>
      </Box>
    );
  }

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', my: 4 }}>
        <CentralizedProgress data-testid="loading-spinner" />
      </Box>
    );
  }

  return (
    <Box sx={{ p: 3 }}>
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'center',
          gap: 1,
          mb: 3,
        }}
      >
        <IconButton
          aria-label="go back"
          onClick={() => {
            if (navigatedFromLink) {
              navigate(navigatedFromLink);
            } else {
              navigate(CATALOG_PATH);
            }
          }}
          sx={{ mr: 2 }}
        >
          <ArrowBackIcon />
        </IconButton>
        <Typography variant="h4" component="h1" sx={{ whiteSpace: 'nowrap' }}>
          Product Catalog Entry:
        </Typography>
        <TextField
          key={id}
          variant="standard"
          defaultValue={id}
          slotProps={{ htmlInput: { sx: { fontSize: 24 } } }}
          fullWidth
          onBlur={(e) => {
            navigateToCatalogIfChanged(e.target.value);
          }}
          onKeyDown={(e) => {
            const target = e.target as HTMLInputElement;
            if (e.key === 'Enter') {
              navigateToCatalogIfChanged(target.value);
            }
          }}
        />
      </Box>

      {!entry ? (
        <AlertWithFeedback
          title="Entry not found!"
          bugErrorMessage={`Product Catalog Entry not found: ${id}`}
        >
          <p>
            The product catalog entry <code>{id}</code> you are looking for was
            not found.
          </p>
        </AlertWithFeedback>
      ) : requestFilingEnabled ? (
        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: { xs: '1fr', lg: '1fr 1fr' },
            gap: 3,
            alignItems: 'start',
          }}
        >
          <Box sx={{ overflowX: 'auto', order: { xs: 2, lg: 1 } }}>
            <MaterialReactTable table={table} />
          </Box>
          <Box sx={{ order: { xs: 1, lg: 2 } }}>
            <OrderForm entry={entry} />
          </Box>
        </Box>
      ) : (
        <Box sx={{ overflowX: 'auto' }}>
          <MaterialReactTable table={table} />
        </Box>
      )}
    </Box>
  );
};

export function Component() {
  const { id = '' } = useParams();
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-product-catalog-details">
      <FleetHelmet
        pageTitle={`${id ? `${id} | ` : ''}Product Catalog Details`}
      />
      <RecoverableErrorBoundary key="fleet-product-catalog-details-page">
        <LoggedInBoundary>
          <ProductCatalogDetailsPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
