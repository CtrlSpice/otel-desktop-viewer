#!/bin/bash

# Launch the app in the background
./otel-desktop-viewer &

# Save the process ID to kill it later
pid=$!

# Wait 2 seconds for everything to boot up
sleep 2

# Send an example trace
curl -i http://localhost:4318/v1/traces -X POST -H "Content-Type: application/json" -d @./.github/workflows/span.json

sleep 1

# Check that a trace summary has been created, and the rootServiceName is correct
response=$(curl 'http://localhost:8000/api/traces' -H "Content-Type: application/json")

echo "$response"

rootServiceName=$(jq '.traceSummaries[0].rootServiceName' <<< $response)

echo "$rootServiceName"

if [ $rootServiceName == "\"test-with-curl\"" ]
then
    echo 'Exit status 0: All good.'
    kill -15 $pid
    exit 0
else
    echo 'Exit status 1: unexpected rootServiceName ' + $rootServiceName
    kill -15 $pid
    exit 1
fi