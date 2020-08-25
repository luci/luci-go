import logging
import os
import subprocess
import time

logging.basicConfig(level=logging.DEBUG)

# web_tests
digest = '1d1e14a2d0da6348f3f37312ef524a2cea1db4ead9ebc6c335f9948ad634cbfd/10430'

# web_tests/fast
# digest = '6bcc0090147aad966ac00aa7423c9b125b6a1b65d73134d5e6a53b2adc4f1c77/7750'

start = time.time()
subprocess.check_call(('./cas download -token-server-host luci-token-server-dev.appspot.com -cas-instance chromium-swarm-dev \
   -digest %s -cache-dir cache -dir hoge -log-level info' % digest).split())
logging.info("finish non-cache download: %s", time.time() - start)

# move instead of remove
os.rename('hoge', 'fuga')

start = time.time()
subprocess.check_call(('./cas download -token-server-host luci-token-server-dev.appspot.com -cas-instance chromium-swarm-dev \
   -digest %s -cache-dir cache -dir hoge -log-level info' % digest).split())
logging.info("finish cached download: %s", time.time() - start)

start = time.time()
subprocess.check_call('./cas archive -token-server-host luci-token-server-dev.appspot.com -cas-instance chromium-swarm-dev \
   -paths .:hoge'.split())
logging.info("finish cached upload: %s", time.time() - start)
