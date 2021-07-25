#!/bin/bash

kubectl get recert.recert5.uberscott.com -o=jsonpath="{.items[0].status.state}"
