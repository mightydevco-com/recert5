#!/bin/bash


cd $(dirname $0)

VERSION=$1

yq() {
  docker run --rm -i -v "${PWD}":/workdir mikefarah/yq yq "$@"
}

yq w -i chart/values.yaml edbOperator quay.io/enterprisedb/edb-operator:$VERSION
yq w -i chart/Chart.yaml version $VERSION
yq w -i chart/Chart.yaml appVersion $VERSION




