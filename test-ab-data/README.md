# Load tests

This folder contains a set of bash scripts used to do a simple (basic) load testing
against ign-fuelserver.

These tests are done using `hey` (a golang tool similar to ApacheBench) command, which concurrently invokes
a given route with parameters.

To install it: from your `<go_wks>/bin/` folder do `go get -u github.com/rakyll/hey`
More info:  https://github.com/rakyll/hey

# Basic Usage

1. Launch your local ign-fuel server (unless you want to target a remote server)

1. Update `env.bash` with your desired values.
1. Then `source env.bash`.
1. Run `run.sh` script.

# Detailed info

1. `parallel-gets.sh` script takes the list of urls from file `get-urls.txt` and
invokes them using ab command.

1. `post-model.sh` is a script that will perform multiple (using `ab`) POST request
to create a model using the model data from `post-model-data.txt`. It is expected
that this request will fail most of the times (due to duplicate model error), but
 in any case it helps to test concurrency on the server.

1. `post-user.sh` is a script that will perform multiple (using `ab`) POST request
to create a user using the data from `post-user-data.txt`. It is expected that
this request will fail most of the times (due to duplicate user error), but in
any case it helps to test concurrency on the server.

1. `run.sh` is a script that just launches at the same time the above 3 scripts.

# Dependencies

This scripts assume the following commands are installed:
`envsubst`, `parallel`, `ab`

`apt-get install parallel`

`apt-get install apache2-utils`
