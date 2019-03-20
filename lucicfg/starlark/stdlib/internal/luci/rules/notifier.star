# Copyright 2019 The LUCI Authors.
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

load('@stdlib//internal/graph.star', 'graph')
load('@stdlib//internal/lucicfg.star', 'lucicfg')
load('@stdlib//internal/validate.star', 'validate')

load('@stdlib//internal/luci/common.star', 'keys', 'kinds')


def _notifier(
      ctx,
      *,
      name=None,

      on_change=None,
      on_failure=None,
      on_new_failure=None,
      on_success=None,

      emails=None,
      notify_blamelist=None,
      blamelist_repos_whitelist=None,

      template=None,

      notified_by=None
  ):
  """Defines a notifier that sends notifications on events from builders.

  A notifier contains a set of conditions on what events are considered
  interesting (e.g. a previously green builder has failed), and a set of
  recipients to notify when an interesting even happens. The conditions are
  specified via `on_*` fields, at least one of which should be set to `True`.

  [Email Templates]: https://chromium.googlesource.com/infra/luci/luci-go/+/master/luci_notify/doc/email_templates.md

  Args:
    name: name of this notifier. Will show up in configs. Required.

    on_change: if True, notify on each change to a build status (e.g. a green
        build becoming red and vice versa). Default is False.
    on_failure: if True, notify on each build failure. Default is False.
    on_new_failure: if True, notify on a build failure unless the previous build
        was a failure too. Default is False.
    on_success: if True, notify on each build success. Default is False.

    emails: an optional list of emails to send notifications to.
    notify_blamelist: if True, send notifications to everyone in the computed
        blamelist for the build. Default is False.
    blamelist_repos_whitelist: an optional list of repository URLs (e.g.
        `https://host/repo`) to restrict the blamelist calculation to. If empty
        (default), only the primary repository associated with the build is
        considered, see `repo` field in luci.builder(...).

    template: name of the template file to use to format the notification email.
        If not present, `default` will be used. See [Email Templates].

    notified_by: builders to receive status notifications from. This relation
        can also be defined via `notifies` filed in luci.builder(...).
  """
  # TODO


notifier = lucicfg.rule(impl = _notifier)
