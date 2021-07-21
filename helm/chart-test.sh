#!/bin/bash
# Note that --chart-dirs does not seem to have any effect,
# which is why I run cd .. &&
docker run --rm -v "$PWD/..:/host" quay.io/helmpack/chart-testing sh -c 'cd /host/helm/charts && ct lint --chart-dirs=/host/helm/charts --charts * --validate-maintainers=false --debug'
