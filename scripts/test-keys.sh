#!/usr/bin/env bash
#
# Generates a new RSA key pair and a test JWT signed with it, and prints the
# environment variables that `go test` needs:
#
#   TOKEN_GENERATOR_PRIVATE_RSA256_KEY  signs the tokens created by the tests
#   TEST_RSA256_PUBLIC_KEY              validates them
#   IGN_TEST_JWT                        a token for the "test-user-identity" user
#
# Usage, from anywhere in the repository:
#
#   eval "$(scripts/test-keys.sh)"
#
# Requires openssl and go. Never use these keys outside of tests.

set -euo pipefail

cd "$(dirname "$0")/.."

dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT

openssl genrsa -out "$dir/key.pem" 2048 2>/dev/null
openssl rsa -in "$dir/key.pem" -pubout -out "$dir/public.pem" 2>/dev/null

# The variables hold the base64 body of each PEM file, on a single line.
private=$(sed '1d;$d' "$dir/key.pem" | tr -d '\n')
public=$(sed '1d;$d' "$dir/public.pem" | tr -d '\n')

jwt=$(TOKEN_GENERATOR_PRIVATE_RSA256_KEY="$private" go run ./cmd/token-generator 2>&1 |
  sed -n 's/.*Signed token: *//p')
if [ -z "$jwt" ]; then
  echo "test-keys.sh: token-generator did not print a token" >&2
  exit 1
fi

echo "export TOKEN_GENERATOR_PRIVATE_RSA256_KEY='$private'"
echo "export TEST_RSA256_PUBLIC_KEY='$public'"
echo "export IGN_TEST_JWT='$jwt'"
