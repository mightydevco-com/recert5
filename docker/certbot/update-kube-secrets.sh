#!/bin/bash

set -e

NEWSECRET=$1

rm /etc/letsencrypt/live/README 2>/dev/null || true

mkdir /ssl2 || true


for dir in /etc/letsencrypt/live/*
  do

    FULLCHAIN="$(realpath $dir/fullchain.pem)"
    PRIVKEY="$(realpath $dir/privkey.pem)"
    
    cp $FULLCHAIN /ssl2/$(basename $dir).crt
    cp $PRIVKEY   /ssl2/$(basename $dir).key

done

kubectl delete secret $NEWSECRET || true
kubectl create secret generic $NEWSECRET --from-file=/ssl2

echo "DONE"
