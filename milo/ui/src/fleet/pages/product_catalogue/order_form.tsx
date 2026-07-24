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

import {
  Box,
  Button,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  TextField,
  Typography,
  Paper,
  Divider,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Link,
} from '@mui/material';
import { DatePicker, LocalizationProvider } from '@mui/x-date-pickers';
import { DateTime } from 'luxon';
import { useState } from 'react';

import { SafeAdapterLuxon } from '@/fleet/adapters/date_adapter';
import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import { ProductCatalogEntry } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export interface OrderFormProps {
  entry: ProductCatalogEntry;
}

const TOOLTIP_CONTAINER_STYLE = {
  position: 'absolute',
  top: -8,
  right: 12,
  zIndex: 1,
  bgcolor: 'background.paper',
  padding: '0 4px',
  display: 'inline-flex',
  alignItems: 'center',
};

export const OrderForm = ({ entry }: OrderFormProps) => {
  const [platform, setPlatform] = useState<string>('');
  const [modalOpen, setModalOpen] = useState(false);
  const [buganizerUrl, setBuganizerUrl] = useState('');
  const [quantity, setQuantity] = useState<number>(1);
  const [resourceGroup, setResourceGroup] = useState<string>('');
  const [estimatedLaunchDate, setEstimatedLaunchDate] =
    useState<DateTime | null>(null);
  const [criticality, setCriticality] = useState<string>('C3');
  const [businessJustification, setBusinessJustification] =
    useState<string>('');

  // TODO: Re-visit this once we add GCE VMs to the catalog.
  const gceVm = 'No';

  // Android specific
  const [hostGroup, setHostGroup] = useState<string>('');
  const [mobileHarness, setMobileHarness] = useState<string>('');
  const [mobileHarnessDimension, setMobileHarnessDimension] =
    useState<string>('');
  const [mobileHarnessWifi, setMobileHarnessWifi] = useState<string>('');
  const [mobileHarnessOwner, setMobileHarnessOwner] = useState<string>('');

  const buildBuganizerUrl = () => {
    let componentId = '';
    let templateId = '';
    const params = new URLSearchParams();

    if (platform === 'OS') {
      componentId = '1092512';
      templateId = '1608417';
    } else if (platform === 'Android') {
      componentId = '1642317';
      templateId = '2040931';
    } else if (platform === 'Browser') {
      componentId = '1261269';
      templateId = '1744377';
    }

    params.append('component', componentId);
    params.append('template', templateId);

    const trimmedProductName = entry.productName.trim();
    const resourceName = (entry.descriptiveName || entry.productName).trim();

    // Set Title
    const title = `[Resource Request] ${quantity} x ${trimmedProductName} for ${resourceGroup}`;
    params.append('title', title);

    // Set Description (initial comment)
    const description =
      `Business Justification: ${businessJustification}\n\n` +
      `Product Catalog Name: ${trimmedProductName}\n` +
      `Catalog ID: ${entry.productCatalogId}`;
    params.append('description', description);

    // Helper to add custom field
    const addCustomField = (fieldId: string, value: string) => {
      if (value) {
        params.append('customFields', `${fieldId}:${value}`);
      }
    };

    // Common Custom Fields
    addCustomField('1320241', platform); // Fulfillment Channel
    addCustomField('1398911', entry.productCatalogId); // Product Catalog ID
    addCustomField('1399654', quantity.toString()); // Resource Quantity
    addCustomField('1473369', resourceGroup); // Resource Group
    if (estimatedLaunchDate) {
      addCustomField('1398937', estimatedLaunchDate.toFormat('M/d/yyyy')); // Estimated Launch Date
    }
    addCustomField('1398961', criticality); // Criticality
    addCustomField('1398886', gceVm); // GCE VM

    // Platform-Specific Custom Fields
    if (platform === 'OS') {
      addCustomField('1374341', resourceName); // Resource Name (OS)
    } else if (platform === 'Android') {
      addCustomField('1374342', resourceName); // Resource Name (Android)
      addCustomField('1399528', hostGroup); // Host Group
      addCustomField('1399763', mobileHarness); // Mobile Harness
      if (mobileHarness === 'Yes') {
        addCustomField('1399551', mobileHarnessDimension); // Dimension
        addCustomField('1399552', mobileHarnessWifi); // WiFi
        addCustomField('1399553', mobileHarnessOwner); // Owner (MDB)
      }
    } else if (platform === 'Browser') {
      addCustomField('1374418', resourceName); // Resource Name (Browser)
    }

    return `https://b.corp.google.com/issues/new?${params.toString()}`;
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const url = buildBuganizerUrl();
    setBuganizerUrl(url);
    setModalOpen(true);
    window.open(url, '_blank', 'noopener,noreferrer');
  };

  return (
    <Paper sx={{ p: 3 }} variant="outlined">
      <Typography variant="h6" gutterBottom>
        Order Resources
      </Typography>
      <Divider sx={{ mb: 2 }} />
      <form onSubmit={handleSubmit}>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
          {/* Row 1: Platform Selection */}
          <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap' }}>
            <Box sx={{ position: 'relative', minWidth: 320 }}>
              <FormControl fullWidth required>
                <InputLabel id="platform-label" required={false}>
                  Platform (Fulfillment Channel)
                </InputLabel>
                <Select
                  labelId="platform-label"
                  value={platform}
                  label="Platform (Fulfillment Channel)"
                  onChange={(e) => {
                    setPlatform(e.target.value);
                  }}
                >
                  <MenuItem value="OS">Chrome OS</MenuItem>
                  <MenuItem value="Android">Android</MenuItem>
                  <MenuItem value="Browser">Browser</MenuItem>
                </Select>
              </FormControl>
              <Box sx={TOOLTIP_CONTAINER_STYLE}>
                <InfoTooltip fontSize="0.875rem">
                  Fulfillment Channel for the requested resources.
                </InfoTooltip>
              </Box>
            </Box>
          </Box>

          {platform && (
            <>
              {/* Row 2: Request Details */}
              <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap' }}>
                <TextField
                  label="Quantity"
                  type="number"
                  value={quantity}
                  onChange={(e) => setQuantity(Number(e.target.value))}
                  inputProps={{ min: 1 }}
                  required
                  sx={{ width: 120 }}
                  InputLabelProps={{ required: false }}
                />

                <Box sx={{ position: 'relative', flex: 1, minWidth: 200 }}>
                  <TextField
                    label="Resource Group"
                    value={resourceGroup}
                    onChange={(e) => setResourceGroup(e.target.value)}
                    required
                    fullWidth
                    InputLabelProps={{ required: false }}
                  />
                  <Box sx={TOOLTIP_CONTAINER_STYLE}>
                    <InfoTooltip fontSize="0.875rem">
                      {"The request's target user/testing group."}
                    </InfoTooltip>
                  </Box>
                </Box>

                <LocalizationProvider dateAdapter={SafeAdapterLuxon}>
                  <DatePicker
                    label="Estimated Launch Date"
                    value={estimatedLaunchDate}
                    onChange={(val) => setEstimatedLaunchDate(val)}
                    minDate={DateTime.now()}
                    slotProps={{
                      textField: {
                        required: true,
                        sx: { flexGrow: 1, minWidth: 200 },
                        InputLabelProps: { required: false },
                      },
                    }}
                  />
                </LocalizationProvider>

                <FormControl sx={{ minWidth: 150 }} required>
                  <InputLabel id="criticality-label" required={false}>
                    Criticality
                  </InputLabel>
                  <Select
                    labelId="criticality-label"
                    value={criticality}
                    label="Criticality"
                    onChange={(e) => setCriticality(e.target.value)}
                  >
                    <MenuItem value="C0">C0</MenuItem>
                    <MenuItem value="C1">C1</MenuItem>
                    <MenuItem value="C2">C2</MenuItem>
                    <MenuItem value="C3">C3</MenuItem>
                  </Select>
                </FormControl>
              </Box>

              {/* Row 2.5: Business Justification */}
              <Box sx={{ display: 'flex', width: '100%' }}>
                <TextField
                  label="Business Justification"
                  value={businessJustification}
                  onChange={(e) => setBusinessJustification(e.target.value)}
                  required
                  fullWidth
                  multiline
                  rows={4}
                  InputLabelProps={{ required: false }}
                />
              </Box>

              {platform === 'Android' && (
                <>
                  <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap' }}>
                    <TextField
                      label="Host Group"
                      value={hostGroup}
                      onChange={(e) => setHostGroup(e.target.value)}
                      required
                      sx={{ minWidth: 200 }}
                      InputLabelProps={{ required: false }}
                    />

                    <Box sx={{ position: 'relative', minWidth: 200 }}>
                      <FormControl fullWidth required>
                        <InputLabel id="mobile-harness-label" required={false}>
                          Mobile Harness
                        </InputLabel>
                        <Select
                          labelId="mobile-harness-label"
                          value={mobileHarness}
                          label="Mobile Harness"
                          onChange={(e) => setMobileHarness(e.target.value)}
                        >
                          <MenuItem value="Yes">Yes</MenuItem>
                          <MenuItem value="No">No</MenuItem>
                        </Select>
                      </FormControl>
                      <Box sx={TOOLTIP_CONTAINER_STYLE}>
                        <InfoTooltip fontSize="0.875rem">
                          Is it a mobile harness resource?
                        </InfoTooltip>
                      </Box>
                    </Box>
                  </Box>

                  {mobileHarness === 'Yes' && (
                    <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap' }}>
                      <Box
                        sx={{ position: 'relative', flex: 1, minWidth: 200 }}
                      >
                        <TextField
                          label="Mobile Harness Dimension (optional)"
                          value={mobileHarnessDimension}
                          onChange={(e) =>
                            setMobileHarnessDimension(e.target.value)
                          }
                          fullWidth
                        />
                        <Box sx={TOOLTIP_CONTAINER_STYLE}>
                          <InfoTooltip fontSize="0.875rem">
                            If Mobile Harness, specify dimensions.
                          </InfoTooltip>
                        </Box>
                      </Box>

                      <Box sx={{ position: 'relative', minWidth: 270 }}>
                        <FormControl fullWidth>
                          <InputLabel id="mobile-harness-wifi-label">
                            Mobile Harness WiFi (optional)
                          </InputLabel>
                          <Select
                            labelId="mobile-harness-wifi-label"
                            value={mobileHarnessWifi}
                            label="Mobile Harness WiFi (optional)"
                            onChange={(e) =>
                              setMobileHarnessWifi(e.target.value)
                            }
                          >
                            <MenuItem value="Yes">Yes</MenuItem>
                            <MenuItem value="No">No</MenuItem>
                          </Select>
                        </FormControl>
                        <Box sx={TOOLTIP_CONTAINER_STYLE}>
                          <InfoTooltip fontSize="0.875rem">
                            If Mobile Harness, is WiFi required?
                          </InfoTooltip>
                        </Box>
                      </Box>

                      <Box
                        sx={{ position: 'relative', flex: 1, minWidth: 200 }}
                      >
                        <TextField
                          label="Mobile Harness Owner (optional)"
                          value={mobileHarnessOwner}
                          onChange={(e) =>
                            setMobileHarnessOwner(e.target.value)
                          }
                          fullWidth
                        />
                        <Box sx={TOOLTIP_CONTAINER_STYLE}>
                          <InfoTooltip fontSize="0.875rem">
                            MDB group owner of the harness.
                          </InfoTooltip>
                        </Box>
                      </Box>
                    </Box>
                  )}
                </>
              )}

              <Box sx={{ display: 'flex', justifyContent: 'flex-end', mt: 1 }}>
                <Button variant="contained" type="submit" size="large">
                  Submit Order
                </Button>
              </Box>
            </>
          )}
        </Box>
      </form>

      <Dialog open={modalOpen} onClose={() => setModalOpen(false)}>
        <DialogTitle>Order Submitted</DialogTitle>
        <DialogContent>
          <Typography gutterBottom>
            A pre-populated Buganizer issue has been generated. Please review
            and complete the submission in the newly opened tab by clicking the{' '}
            <strong>Create</strong> button.
          </Typography>
          <Typography gutterBottom sx={{ mt: 2 }}>
            If the Buganizer page did not open automatically (e.g., if it was
            blocked by a popup blocker), you can{' '}
            <Link href={buganizerUrl} target="_blank" rel="noopener noreferrer">
              click here to open it manually
            </Link>
            .
          </Typography>
          <Typography gutterBottom sx={{ mt: 2 }}>
            Once the request is created, you can monitor its fulfillment status
            on the{' '}
            <Link href="/ui/fleet/labs/requests" target="_blank">
              Resource Request Insights (RRI)
            </Link>{' '}
            page.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setModalOpen(false)} variant="contained">
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </Paper>
  );
};
