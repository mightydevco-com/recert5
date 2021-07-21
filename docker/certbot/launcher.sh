#!/bin/bash

COMMAND=$1
echo "command: $COMMAND"

if [ "$COMMAND" = "create" ]; then
  shift
  ./create.sh $@
  exit $?
elif [ "$COMMAND" = "idle" ]; then
  echo "idling..."
  tail -f /dev/null
  exit $?
elif [ "$COMMAND" = "dryrun" ]; then
  shift
  ./dryrun.sh $@
  exit $?
elif [ "$COMMAND" = "mock" ]; then
  shift
  ./mock.sh $@
  exit $?
elif [ "$COMMAND" = "fail" ]; then
  shift
  ./fail.sh $@
  exit $?
else
  exit 1
fi 



echo "cannot process command: $COMMAND"
