#!/usr/bin/env bash
set -o verbose
set -eu
set -o pipefail
export PROXY_PATH_MAIN_TARGET=${CACHE_URL}
/opt/domainproxy-server -Djavax.net.ssl.trustStore=$JAVA_HOME/lib/security/cacerts &
TASK="ip link set dev lo up && /original-content/build.sh $@"
unshare -n -Ufp -r --  sh -c "$TASK"
