# Idea taken from here:
# https://github.com/docker-library/docs/blob/6b6b3f34023ab90821453ed1e88e7e9165c6b0d1/.template-helpers/variant-onbuild.md

FROM golang:1.14.2

RUN apt-get update && apt-get install -y nano vim &&  \
  git config --global user.name "gz-fuelserver"  &&  \
  git config --global user.email "gz-fuelserver@test.org"

COPY . /root/gz-fuelserver
WORKDIR /root/gz-fuelserver

# Build app
RUN go build
CMD ["./fuel-server"]

EXPOSE 8000
