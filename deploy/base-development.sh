#!/bin/sh

NAMESPACE=$1
if [ "$NAMESPACE" = "" ]; then
    NAMESPACE=test-jvm-namespace
fi

kubectl create namespace $NAMESPACE
kubectl create namespace jvm-build-service

kubectl delete --ignore-not-found deployments.apps hacbs-jvm-operator -n jvm-build-service
# we don't restart the cache and local storage by default
# for most cases in development this is not necessary, and just slows things
# down by needing things to be re-cached/rebuilt

kubectl delete --ignore-not-found deployments.apps jvm-build-workspace-artifact-cache


DIR=`dirname $0`
kubectl config set-context --current --namespace=$NAMESPACE
kubectl delete --ignore-not-found secret jvm-build-image-secrets jvm-build-git-secrets
kubectl create secret generic jvm-build-image-secrets --from-file=.dockerconfigjson=$HOME/.docker/config.json --type=kubernetes.io/dockerconfigjson
kubectl create secret generic jvm-build-git-secrets --from-literal .git-credentials="
https://$GITHUB_E2E_ORGANIZATION:$GITHUB_TOKEN@github.com
https://test:test@gitlab.com
"

JVM_BUILD_SERVICE_IMAGE=quay.io/$QUAY_USERNAME/hacbs-jvm-controller \
JVM_BUILD_SERVICE_CACHE_IMAGE=quay.io/$QUAY_USERNAME/hacbs-jvm-cache \
JVM_BUILD_SERVICE_REQPROCESSOR_IMAGE=quay.io/$QUAY_USERNAME/hacbs-jvm-build-request-processor:dev \
$DIR/patch-yaml.sh
kubectl apply -k $DIR/overlays/development

