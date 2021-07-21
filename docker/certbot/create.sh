#!/bin/bash

echo $@

set -e

DOMAIN=$1
EMAIL=$2
SECRET=$3

certbot certonly --standalone -d $DOMAIN --email $EMAIL --non-interactive --agree-tos

./update-kube-secrets.sh $SECRET-new
