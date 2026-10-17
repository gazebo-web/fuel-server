# The AI Builder mesh worker (plan §5.1, §5.7): the Go front end plus the
# gzmesh pipeline, each job in a bubblewrap jail. Build from the
# fuel-server root:
#
#   docker build -f meshworker/Dockerfile -t gazebo/meshworker .
#
# bubblewrap needs to create user, mount and network namespaces and mount
# a fresh /proc inside the container. Docker's default seccomp and AppArmor
# profiles and its masked /proc and /sys block that, so they are relaxed
# below; no capabilities are needed. In production run the container under
# gVisor (--runtime=runsc), which contains whatever the relaxed profiles
# would otherwise expose to the host kernel:
#
#   docker run --rm -p 8020:8020 --read-only --tmpfs /tmp --cap-drop ALL \
#     --security-opt no-new-privileges --security-opt seccomp=unconfined \
#     --security-opt apparmor=unconfined --security-opt systempaths=unconfined \
#     --pids-limit 512 --memory 8g --cpus 4 -e MESHWORKER_TOKEN=... gazebo/meshworker
#
# Verified 2026-10-17 under runc on Ubuntu 26.04 (whose AppArmor restricts
# unprivileged user namespaces): the cone job builds in about 3 s.

FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/meshworker ./cmd/meshworker
RUN CGO_ENABLED=0 go build -trimpath -o /meshworker ./cmd/meshworker

FROM python:3.12-slim-bookworm
# bubblewrap is the per-job jail; util-linux provides prlimit; libgl1 and
# libxrender1 are the X/GL stubs OCCT's wheel links against.
RUN apt-get update \
 && apt-get install -y --no-install-recommends bubblewrap util-linux libgl1 libxrender1 \
 && rm -rf /var/lib/apt/lists/*
COPY meshworker/requirements.txt /opt/meshworker/requirements.txt
RUN grep -v '^pytest' /opt/meshworker/requirements.txt > /tmp/req.txt \
 && pip install --no-cache-dir -r /tmp/req.txt \
 && rm /tmp/req.txt
COPY meshworker/gzmesh /opt/meshworker/gzmesh
COPY --from=build /meshworker /usr/local/bin/meshworker
RUN python -m compileall -q /opt/meshworker/gzmesh \
 && useradd --system --uid 10001 --no-create-home meshworker
USER meshworker
ENV MESHWORKER_PYTHON=/usr/local/bin/python \
    MESHWORKER_GZMESH=/opt/meshworker \
    MESHWORKER_WORKDIR=/tmp/meshworker \
    MESHWORKER_ADDR=:8020
EXPOSE 8020
ENTRYPOINT ["/usr/local/bin/meshworker"]
