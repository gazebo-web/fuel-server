# Idea taken from here:
# https://github.com/docker-library/docs/blob/6b6b3f34023ab90821453ed1e88e7e9165c6b0d1/.template-helpers/variant-onbuild.md

FROM golang:1.19

RUN apt-get update && apt-get install -y nano vim &&  \
  git config --global user.name "ign-fuelserver"  &&  \
  git config --global user.email "ign-fuelserver@test.org"

WORKDIR /go/src/gitlab.com/ignitionrobotics/web/fuelserver/

COPY . .

RUN go mod download

CMD go test -v ./...