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

import CloudOutlinedIcon from '@mui/icons-material/CloudOutlined';
import DevicesIcon from '@mui/icons-material/Devices';
import DnsIcon from '@mui/icons-material/Dns';
import LaptopIcon from '@mui/icons-material/Laptop';
import PhoneAndroidIcon from '@mui/icons-material/PhoneAndroid';
import { Box } from '@mui/material';

import { fleetColors } from '@/fleet/theme/tokens';

import { resolveProductType } from './catalog_details_helpers';

const CATEGORY_ILLUSTRATION_ICON_SIZE = 40;

const PRODUCT_ICON_SX = {
  fontSize: CATEGORY_ILLUSTRATION_ICON_SIZE,
  color: fleetColors.primary,
  flexShrink: 0,
  display: 'block',
} as const;

export interface ProductTypeIconProps {
  readonly productType?: string;
}

export const ProductTypeIcon = ({ productType }: ProductTypeIconProps) => {
  const prodType = (productType || '').trim().toLowerCase();
  switch (prodType) {
    case 'android-testbed':
      return <PhoneAndroidIcon sx={PRODUCT_ICON_SX} />;
    case 'os-testbed':
      return <DnsIcon sx={PRODUCT_ICON_SX} />;
    case 'gce':
      return <CloudOutlinedIcon sx={PRODUCT_ICON_SX} />;
    case 'hardware':
      return <LaptopIcon sx={PRODUCT_ICON_SX} />;
    case 'peripherals':
    default:
      return <DevicesIcon sx={PRODUCT_ICON_SX} />;
  }
};

export interface CategoryIllustrationProps {
  readonly productType?: string;
  readonly productCatalogId: string;
}

export const CategoryIllustration = ({
  productType,
  productCatalogId,
}: CategoryIllustrationProps) => {
  const resolvedType = resolveProductType(productType, productCatalogId);
  return (
    <Box
      data-testid={`catalog-photo-fallback-${productCatalogId}`}
      sx={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        my: 'auto',
      }}
    >
      <ProductTypeIcon productType={resolvedType} />
    </Box>
  );
};
