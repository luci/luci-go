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

import styled from '@emotion/styled';
import DevicesIcon from '@mui/icons-material/Devices';
import DnsIcon from '@mui/icons-material/Dns';
import LaptopIcon from '@mui/icons-material/Laptop';
import PhoneAndroidIcon from '@mui/icons-material/PhoneAndroid';
import {
  Box,
  Card,
  CardContent,
  Chip,
  Grid,
  Skeleton,
  TablePagination,
  Typography,
} from '@mui/material';
import { MRT_PaginationState, MRT_Updater } from 'material-react-table';
import { Link } from 'react-router';

import { colors } from '@/fleet/theme/colors';
import { ProductCatalogEntry } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

enum PlmStatus {
  GA = 'GA',
  LA = 'LA',
  NPI = 'NPI',
}

enum ProductType {
  ANDROID_TESTBED = 'android-testbed',
  OS_TESTBED = 'os-testbed',
  HARDWARE = 'hardware',
  PERIPHERALS = 'peripherals',
}

const getPlmStatusColor = (status?: string) => {
  switch (status?.toUpperCase()) {
    case PlmStatus.GA:
      return {
        bg: colors.green[50],
        text: colors.green[700],
        border: colors.green[200],
      };
    case PlmStatus.LA:
      return {
        bg: colors.orange[50],
        text: colors.orange[700],
        border: colors.orange[200],
      };
    case PlmStatus.NPI:
      return {
        bg: colors.blue[50],
        text: colors.blue[700],
        border: colors.blue[200],
      };
    default:
      return {
        bg: colors.grey[50],
        text: colors.grey[700],
        border: colors.grey[200],
      };
  }
};

const getProductIcon = (entry: ProductCatalogEntry) => {
  const prodType = (entry.productType || '').trim().toLowerCase();

  switch (prodType) {
    case ProductType.ANDROID_TESTBED:
      return (
        <PhoneAndroidIcon sx={{ fontSize: 40, color: colors.blue[700] }} />
      );
    case ProductType.OS_TESTBED:
      return <DnsIcon sx={{ fontSize: 40, color: colors.blue[700] }} />;
    case ProductType.HARDWARE:
      return <LaptopIcon sx={{ fontSize: 40, color: colors.blue[700] }} />;
    case ProductType.PERIPHERALS:
    default:
      return <DevicesIcon sx={{ fontSize: 40, color: colors.blue[700] }} />;
  }
};

const DEFAULT_PAGE_SIZE_OPTIONS = [20, 50, 100];

const CardWrapper = styled(Card)`
  display: flex;
  height: 100%;
  border: 1px solid ${colors.grey[200]};
  border-radius: 8px;
  background-color: ${colors.white};
  transition:
    transform 0.2s ease-in-out,
    box-shadow 0.2s ease-in-out,
    border-color 0.2s ease-in-out;
  cursor: pointer;
  text-decoration: none;
  color: inherit;

  &:hover {
    transform: translateY(-4px);
    box-shadow: 0 8px 16px rgba(0, 0, 0, 0.08);
    border-color: ${colors.blue[200]};
  }
`;

const ImageSection = styled(Box)`
  width: 140px;
  min-width: 140px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(
    135deg,
    ${colors.blue[50]} 0%,
    ${colors.blue[100]} 100%
  );
  border-right: 1px solid ${colors.grey[200]};
  border-top-left-radius: 8px;
  border-bottom-left-radius: 8px;
`;

const ContentSection = styled(CardContent)`
  flex-grow: 1;
  padding: 16px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  &:last-child {
    padding-bottom: 16px;
  }
`;

interface ProductCatalogueCardViewProps {
  entries: ProductCatalogEntry[];
  isLoading: boolean;
  pagination: MRT_PaginationState;
  onPaginationChange: (updater: MRT_Updater<MRT_PaginationState>) => void;
  totalCount: number;
}

const getPathnameWithParams = () => {
  return window.location.href.toString().split(window.location.host)[1];
};

export const ProductCatalogueCardView = ({
  entries,
  isLoading,
  pagination,
  onPaginationChange,
  totalCount,
}: ProductCatalogueCardViewProps) => {
  if (isLoading) {
    return (
      <Box sx={{ mt: 2 }}>
        <Grid container spacing={3}>
          {Array.from({ length: pagination.pageSize }).map((_, idx) => (
            <Grid item xs={12} md={6} key={idx}>
              <Card sx={{ display: 'flex', height: 160, borderRadius: 2 }}>
                <Skeleton variant="rectangular" width={140} height="100%" />
                <Box
                  sx={{
                    flexGrow: 1,
                    p: 2,
                    display: 'flex',
                    flexDirection: 'column',
                    justifyContent: 'space-between',
                  }}
                >
                  <Box>
                    <Skeleton variant="text" width="60%" height={28} />
                    <Skeleton variant="text" width="40%" height={20} />
                  </Box>
                  <Box sx={{ display: 'flex', gap: 2 }}>
                    <Skeleton variant="rectangular" width={80} height={24} />
                    <Skeleton variant="rectangular" width={80} height={24} />
                    <Skeleton variant="rectangular" width={80} height={24} />
                  </Box>
                </Box>
              </Card>
            </Grid>
          ))}
        </Grid>
      </Box>
    );
  }

  if (entries.length === 0) {
    return (
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          py: 8,
          px: 2,
          border: `1px dashed ${colors.grey[300]}`,
          borderRadius: 2,
          backgroundColor: colors.grey[50],
        }}
      >
        <DevicesIcon sx={{ fontSize: 48, color: colors.grey[400], mb: 2 }} />
        <Typography
          variant="h6"
          sx={{ color: colors.grey[800], mb: 1, fontWeight: 600 }}
        >
          No catalog entries found
        </Typography>
        <Typography
          variant="body2"
          sx={{ color: colors.grey[600], textAlign: 'center' }}
        >
          Try adjusting your search or filters to find what you&apos;re looking
          for.
        </Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ mt: 2 }}>
      <Grid container spacing={3}>
        {entries.map((entry) => (
          <Grid item xs={12} md={6} key={entry.productCatalogId}>
            <Link
              to={`/ui/fleet/catalog/${entry.productCatalogId}`}
              state={{
                navigatedFromLink: getPathnameWithParams(),
              }}
              style={{ textDecoration: 'none', color: 'inherit' }}
            >
              <CardWrapper>
                <ImageSection>{getProductIcon(entry)}</ImageSection>
                <ContentSection>
                  <Box>
                    <Box
                      sx={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        alignItems: 'flex-start',
                        mb: 0.5,
                      }}
                    >
                      <Typography
                        variant="h6"
                        sx={{
                          fontSize: '1rem',
                          fontWeight: 600,
                          color: colors.grey[900],
                          lineHeight: 1.2,
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          display: '-webkit-box',
                          WebkitLineClamp: 2,
                          WebkitBoxOrient: 'vertical',
                        }}
                      >
                        {entry.productName || entry.productCatalogId}
                      </Typography>
                      {entry.fleetPlmStatus && (
                        <Chip
                          label={entry.fleetPlmStatus}
                          size="small"
                          sx={{
                            ml: 1,
                            fontWeight: 600,
                            fontSize: '0.75rem',
                            height: 20,
                            backgroundColor: getPlmStatusColor(
                              entry.fleetPlmStatus,
                            ).bg,
                            color: getPlmStatusColor(entry.fleetPlmStatus).text,
                            border: `1px solid ${
                              getPlmStatusColor(entry.fleetPlmStatus).border
                            }`,
                          }}
                        />
                      )}
                    </Box>
                    <Typography
                      variant="caption"
                      sx={{
                        color: colors.grey[600],
                        display: 'block',
                        mb: 1.5,
                        fontFamily: 'monospace',
                      }}
                    >
                      ID: {entry.productCatalogId}
                    </Typography>
                    {entry.descriptiveName && (
                      <Typography
                        variant="body2"
                        sx={{
                          color: colors.grey[700],
                          mb: 2,
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          display: '-webkit-box',
                          WebkitLineClamp: 2,
                          WebkitBoxOrient: 'vertical',
                          fontSize: '0.8125rem',
                        }}
                      >
                        {entry.descriptiveName}
                      </Typography>
                    )}
                  </Box>

                  <Box
                    sx={{
                      display: 'flex',
                      flexWrap: 'wrap',
                      gap: 3,
                      pt: 1.5,
                      borderTop: `1px solid ${colors.grey[100]}`,
                    }}
                  >
                    {entry.gpn && (
                      <Box>
                        <Typography
                          variant="caption"
                          sx={{ color: colors.grey[500], display: 'block' }}
                        >
                          GPN
                        </Typography>
                        <Typography
                          variant="body2"
                          sx={{
                            fontWeight: 500,
                            color: colors.grey[800],
                            fontSize: '0.8125rem',
                          }}
                        >
                          {entry.gpn}
                        </Typography>
                      </Box>
                    )}
                    {entry.resourceType && (
                      <Box>
                        <Typography
                          variant="caption"
                          sx={{ color: colors.grey[500], display: 'block' }}
                        >
                          Resource Type
                        </Typography>
                        <Typography
                          variant="body2"
                          sx={{
                            fontWeight: 500,
                            color: colors.grey[800],
                            fontSize: '0.8125rem',
                          }}
                        >
                          {entry.resourceType}
                        </Typography>
                      </Box>
                    )}
                    {entry.r11n && entry.r11n.length > 0 && (
                      <Box>
                        <Typography
                          variant="caption"
                          sx={{ color: colors.grey[500], display: 'block' }}
                        >
                          R11N
                        </Typography>
                        <Typography
                          variant="body2"
                          sx={{
                            fontWeight: 500,
                            color: colors.grey[800],
                            fontSize: '0.8125rem',
                          }}
                        >
                          {entry.r11n.join(', ')}
                        </Typography>
                      </Box>
                    )}
                    {entry.unitCost !== undefined &&
                      entry.unitCost !== '' &&
                      entry.unitCost !== '0' && (
                        <Box>
                          <Typography
                            variant="caption"
                            sx={{ color: colors.grey[500], display: 'block' }}
                          >
                            Unit Cost
                          </Typography>
                          <Typography
                            variant="body2"
                            sx={{
                              fontWeight: 500,
                              color: colors.grey[800],
                              fontSize: '0.8125rem',
                            }}
                          >
                            ${entry.unitCost}
                          </Typography>
                        </Box>
                      )}
                    {entry.numberOfDevicesPerRack !== undefined &&
                      entry.numberOfDevicesPerRack !== 0 && (
                        <Box>
                          <Typography
                            variant="caption"
                            sx={{ color: colors.grey[500], display: 'block' }}
                          >
                            Per Rack
                          </Typography>
                          <Typography
                            variant="body2"
                            sx={{
                              fontWeight: 500,
                              color: colors.grey[800],
                              fontSize: '0.8125rem',
                            }}
                          >
                            {entry.numberOfDevicesPerRack}
                          </Typography>
                        </Box>
                      )}
                  </Box>
                </ContentSection>
              </CardWrapper>
            </Link>
          </Grid>
        ))}
      </Grid>

      <TablePagination
        component="div"
        count={totalCount}
        page={pagination.pageIndex}
        onPageChange={(_, newPage) => {
          onPaginationChange((prev) => ({ ...prev, pageIndex: newPage }));
        }}
        rowsPerPage={pagination.pageSize}
        onRowsPerPageChange={(e) => {
          const newPageSize = parseInt(e.target.value, 10);
          onPaginationChange((prev) => ({
            ...prev,
            pageIndex: 0,
            pageSize: newPageSize,
          }));
        }}
        rowsPerPageOptions={DEFAULT_PAGE_SIZE_OPTIONS}
      />
    </Box>
  );
};
