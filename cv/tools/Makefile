# Copyright 2021 The LUCI Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


TOKEN=$(shell luci-auth token -scopes https://www.googleapis.com/auth/userinfo.email)
DEV=luci-change-verifier-dev
PROD=luci-change-verifier

flush-redis-dev:
	@curl -H "Authorization:Bearer $(TOKEN)" https://$(DEV).appspot.com/internal/admin/flush-redis

# Use different starting prefix for prod to avoid accidentally hitting it via
# completion when -dev was intended (based on tandrii@ own sad experience).
prod-flush-redis:
	@curl -H "Authorization:Bearer $(TOKEN)" https://$(PROD).appspot.com/internal/admin/flush-redis
