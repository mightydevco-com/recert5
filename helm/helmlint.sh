#!/bin/bash


cd `dirname $0`


set -e
for d in */chart/; do
  echo "---helm lint on $PWD/$d---"
  helm lint --strict "$PWD/$d"
done
