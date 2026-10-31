FROM golang:1.27-trixie AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/fuel-server .

# The server shells out to git (init, add, commit, tag, show, archive, clone and
# rev-list), so the runtime image keeps the git CLI. media-types provides
# /etc/mime.types, which sets the Content-Type of file downloads.
FROM debian:trixie-slim

# The server runs as this fixed UID, which must own everything in
# IGN_FUEL_RESOURCE_DIR (/fuel on Elastic Beanstalk). See "Running as a
# non-root user" in the README.
ARG FUEL_UID=10001

RUN apt-get update && \
  apt-get install -y --no-install-recommends ca-certificates git media-types && \
  rm -rf /var/lib/apt/lists/* && \
  git config --system user.name "gz-fuelserver" && \
  git config --system user.email "gz-fuelserver@test.org" && \
  git config --system --add safe.directory '*' && \
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
