# Idea taken from here:
# https://github.com/docker-library/docs/blob/6b6b3f34023ab90821453ed1e88e7e9165c6b0d1/.template-helpers/variant-onbuild.md

FROM golang:1.27.2

RUN apt-get update && apt-get install -y nano vim &&  \
  git config --global user.name "gz-fuelserver"  &&  \
  git config --global user.email "gz-fuelserver@test.org"

COPY . /root/gz-fuelserver
WORKDIR /root/gz-fuelserver

# Build app. The image holds both binaries: fuel-server (the default) and
# the AI Builder agent, run as a second container on the same host with
# command "aibuilder" (port 8010).
RUN go build && go build -o /usr/local/bin/aibuilder ./cmd/aibuilder
CMD ["./fuel-server"]

EXPOSE 8000 8010
