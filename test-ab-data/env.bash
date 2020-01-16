# Apache AB arguments:
# -c arg for ab (concurrency. Number of multiple requests to make at a time). eg. 50
export AB_C=50
# -n arg for ab (number of requests to perform). eg. 200
export AB_N=400

# Test token -- the one used in tests
# Note: you should launch your local server using the corresponding Test private key.
export AB_AUTH_HEADER=eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0LXVzZXItaWRlbnRpdHkifQ.iV59-kBkZ86XKKsph8fxEeyxDiswY1zvPGi4977cHbbDEkMA3Y3t_zzmwU4JEmjbTeToQZ_qFNJGGNufK2guLy0SAicwjDmv-3dHDfJUH5x1vfi1fZFnmX_b8215BNbCBZU0T2a9DEFypxAQCQyiAQDE9gS8anFLHHlbcWdJdGw

# If you want to test against a public server, eg staging, you need to put here a valid jwt token
# export AB_AUTH_HEADER= <valid-jwt-token>

# Server to test (eg. https://staging-api.ignitionfuel.org)
export AB_SERVER=http://localhost:8000
