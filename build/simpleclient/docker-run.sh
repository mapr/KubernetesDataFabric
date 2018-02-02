#!/bin/sh

docker run -d \
  --env-file ./env.list \
  -t "$*"
