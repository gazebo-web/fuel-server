# Apache AB arguments:
# -c arg for ab (concurrency. Number of multiple requests to make at a time). eg. 50
export AB_C=50
# -n arg for ab (number of requests to perform). eg. 200
export AB_N=400

# Test token -- the one used in tests (see scripts/test-keys.sh).
# Note: you should launch your local server using the corresponding Test public key.
export AB_AUTH_HEADER=${IGN_TEST_JWT}

# If you want to test against a public server, eg staging, you need to put here a valid jwt token
# export AB_AUTH_HEADER= <valid-jwt-token>

# Server to test (eg. https://staging-api.ignitionfuel.org)
export AB_SERVER=http://localhost:8000
