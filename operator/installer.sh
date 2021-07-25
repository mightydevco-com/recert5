#!/bin/bash

mkdir ../docs/static/files
kustomize build config/default/ > ../docs/static/files/recert5-operator.yaml
