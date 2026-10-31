FROM golang:1.27-trixie AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/fuel-server .

# The server runs git commands for every repository on disk, so the runtime
# image keeps the git CLI.
FROM debian:trixie-slim

# The server runs as this fixed UID, which must own everything in
# IGN_FUEL_RESOURCE_DIR (/fuel on Elastic Beanstalk). See "Running as a
# non-root user" in the README.
ARG FUEL_UID=10001

RUN apt-get update && \
  apt-get install -y --no-install-recommends ca-certificates git && \
  rm -rf /var/lib/apt/lists/* && \
  git config --system user.name "gz-fuelserver" && \
  git config --system user.email "gz-fuelserver@test.org" && \
  groupadd --gid ${FUEL_UID} fuel && \
  useradd --uid ${FUEL_UID} --gid fuel --home-dir /nonexistent --no-create-home \
    --shell /usr/sbin/nologin fuel

WORKDIR /app
COPY --from=build /out/fuel-server ./
COPY swagger.json validators_owners_blacklist.json ./
COPY templates ./templates
COPY permissions/policy.conf ./permissions/

USER ${FUEL_UID}:${FUEL_UID}

EXPOSE 8000
CMD ["./fuel-server"]
