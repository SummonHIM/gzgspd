#!/bin/bash

if [ ! -d "built" ]; then
    mkdir -vp "built"
fi

if [[ "$RUNNER_OS" == "Windows" ]]; then
  export OUTFILE="built/gzgspd-$RUNNER_OS.exe"
else
  export OUTFILE="built/gzgspd-$RUNNER_OS"
fi

for env in $BUILD_ENVS; do
  # shellcheck disable=SC2163
  export "$env"
  echo "$env" > "$GITHUB_ENV"
done
echo "OUTFILE=$OUTFILE" > "$GITHUB_ENV"