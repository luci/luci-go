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

/**
 * URLs used to test URL version migration.
 */
export const RRI_VERSION_TEST_URLS = {
  v1: 'http://localhost:8080/ui/fleet/requests?filters=rr_id%3D%28%22RR-001%22+OR+%22RR-0012%22%29%26resource_details%3D%28%22FF+Creation+Testing+-+%5BResource+Request+Demo%5D%5BAnnual%5D%3A+Pixel+Tablet+%282023%29+x+549+units+for+xTS+Quality+Improvement+-+Devices+for+automated+testing%22+OR+%22FF+Creation+Testing+2+-+%5BResource+Request+Demo%5D%5BAnnual%5D%3A+Pixel+Tablet+%282023%29+x+451+units+for+xTS+Quality+Improvement+-+Devices+for+automated+testing%22%29%26fulfillment_status%3D%22NOT_STARTED%22%26resource_request_target_delivery_date_min%3D2025-01-01%26resource_request_target_delivery_date_max%3D2026-01-01%26material_sourcing_actual_delivery_date_min%3D2026-01-01%26build_actual_delivery_date_max%3D2026-01-01%26customer%3D%28%22ANPLAT%22+OR+%22BROTHER%22%29%26resource_groups%3D%28%22ANDROID_TABLET%22+OR+%22CHROME_DESKTOP%22%29',
};
