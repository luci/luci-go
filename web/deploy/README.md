## /web/deploy

This directory contains symlinks to `/web/web.py` that are used by the
`luci_deploy` tool for initialization (initialize.py) and web app building
(build.py). `web.py` uses special `luci_deploy` argument layouts when
invoked from one of these symlink scripts.
