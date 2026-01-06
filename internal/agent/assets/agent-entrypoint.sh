#!/bin/bash
if [ $# -eq 0 ]; then
  exec /bin/bash --login -i
else
  exec /bin/bash --login -c "$*"
fi
