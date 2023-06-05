#!/bin/bash
cat >>Dockerfile <<RHTAPEOFMARKERFORHEREDOC
FROM quay.io/redhat-appstudio/hacbs-jdk8-builder:13f1e4ed67a29061e033e251c9f38e7905279649
as git
RHTAPEOFMARKERFORHEREDOC
echo -n 'RUN echo ' >>Dockerfile
cat <<RHTAPEOFMARKERFORHEREDOC | base64 >>Dockerfile
git clone https://github.com/wildfly/wildfly-common.git
/workspace/source/workspace && cd /workspace/source/workspace && git
reset --hard 79612525dcf82d28dab8065965d80f5a453481ad && git submodule
init && git submodule update --recursive
#!/usr/bin/env bash
set -o
verbose
set -eu
set -o pipefail

cat >\"/workspace/build-settings\"/settings.xml
<<EOF
<settings>
  <mirrors>
    <mirror>
      <id>mirror.default</id>
     <url>https://jvm-build-workspace-artifact-cache-tls.test-jvm-namespace.svc.cluster.local/v2/cache/rebuild/1583527756000</url>
     <mirrorOf>*</mirrorOf>
    </mirror>
  </mirrors>
</settings>
EOF
RHTAPEOFMARKERFORHEREDOC
echo -n ' | base64 -d >script.sh' >>Dockerfile
RUN script.sh >>Dockerfile
cat Dockerfile
