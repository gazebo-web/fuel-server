#!/bin/bash
# NOTE: update the post-logfile-data.txt with the name of the participant team

# Note: Put here the JWT token to use with http requests.
# This token should belong to a member of the Participant team set in "post-logfile-data.txt"
export AB_AUTH_HEADER=

# Server to test (eg. https://staging-api.ignitionfuel.org OR http://localhost:8000)
export AB_SERVER=https://staging-api.ignitionfuel.org

# -c arg for ab (concurrency. Number of multiple requests to make at a time). eg. 50
export AB_C=10 #50
# -n arg for ab (number of requests to perform). eg. 200
export AB_N=100 #400

hey -c $AB_C -n $AB_N -T "multipart/form-data; boundary=---------------------------103633748619302808251276201494" -m POST -D post-logfile-data.txt -H "Authorization: Bearer ${AB_AUTH_HEADER}" $AB_SERVER/1.0/subt/logfiles
