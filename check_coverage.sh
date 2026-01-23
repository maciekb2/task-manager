#!/bin/bash

MODULES="enricher client worker server result-store deadletter scheduler notifier audit ingest visualizer"

echo "| Module | Coverage |"
echo "|---|---|"

for mod in $MODULES; do
    if [ -d "$mod" ]; then
        cd $mod
        go mod tidy > /dev/null 2>&1
        out=$(go test -coverprofile=coverage.out . 2>&1)
        if [ $? -ne 0 ]; then
             cov="FAIL"
        else
             cov=$(go tool cover -func=coverage.out | grep total: | awk '{print $3}')
        fi
        echo "| $mod | $cov |"
        rm -f coverage.out
        cd ..
    fi
done
