# Apache AB arguments:
# -c arg for ab (concurrency. Number of multiple requests to make at a time). eg. 50
export AB_C=50
# -n arg for ab (number of requests to perform). eg. 200
export AB_N=400

# The test token from scripts/test-keys.sh. Start the local server with
# AUTH0_RSA256_PUBLIC_KEY set to the matching TEST_RSA256_PUBLIC_KEY.
export AB_AUTH_HEADER=${IGN_TEST_JWT}

# If you want to test against a public server, eg staging, you need to put here a valid jwt token
# export AB_AUTH_HEADER= <valid-jwt-token>

# Server to test (eg. https://staging-api.ignitionfuel.org)
export AB_SERVER=http://localhost:8000
