# Idea taken from here:
# https://github.com/docker-library/docs/blob/6b6b3f34023ab90821453ed1e88e7e9165c6b0d1/.template-helpers/variant-onbuild.md

FROM golang:1.9.4

RUN apt-get update && apt-get install -y nano vim &&  \
  git config --global user.name "ign-fuelserver"  &&  \
  git config --global user.email "ign-fuelserver@test.org"

RUN mkdir -p /go/src/bitbucket.org/ignitionrobotics/ign-fuelserver
COPY . /go/src/bitbucket.org/ignitionrobotics/ign-fuelserver
WORKDIR /go/src/bitbucket.org/ignitionrobotics/ign-fuelserver

# Install go dep
RUN curl -fsSL -o /usr/local/bin/dep https://github.com/golang/dep/releases/download/v0.4.1/dep-linux-amd64 && chmod +x /usr/local/bin/dep
# install the dependencies without checking for go code
RUN dep ensure -vendor-only

# Build app
RUN go install
CMD ["/go/bin/ign-fuelserver"]

EXPOSE 8000
