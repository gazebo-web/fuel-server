<div align="center">
  <img src="./assets/logo.png" width="200" alt="Ignition Robotics" />
  <h1>Ignition Robotics</h1>
  <p>Ignition Web Fuel Server hosts simulation resources for consumption through a REST API.</p>
</div>

# Install

1. Dependencies

    1. Go version 1.9 or above (NOTE: we are currently using 1.9.4)

        * On Ubuntu Xenial or earlier, download and follow instructions from https://golang.org/dl/

        * On Ubuntu Bionic:

            ```
            sudo apt-get update
            sudo apt-get install golang-go
            ```

    1. Other dependencies

        ```
        sudo apt-get update
        sudo apt-get install golang-goprotobuf-dev git
        ```

1. Make sure your git config is set, i.e.

        git config --global user.name "User Name"
        git config --global user.email "user@email.com"

1. Make a workspace, for example

    ```
    mkdir -p ~/go_ws
    ```

1. Download server code into new directories in the workspace:

    ```
    git clone https:/gitlab.com/ignitionrobotics/web/fuelserver ~/go_ws/src/gitlab.com/ignitionrobotics/web/fuelserver
    ```

1. Set necessary environment variable (needs to be set every time the environment is built)

    ```
    export GOPATH=~/go_ws
    ```

1. Install dep tool

    Create a bin directory

    ```
    mkdir ~/go_ws/bin
    ```

    Move to the workspace's root

    ```
    cd ~/go_ws
    ```

    Install dep tool
    
    We're using version 0.4.1 to manage dependencies.

    > On Ubuntu Bionic, the dep tool will be installed under ~/go_ws/bin (`GOBIN`), so create that:
    >     `mkdir -p ~/go_ws/bin`

    ```
    export DEP_RELEASE_TAG=v0.4.1
    curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh
    ```

1. Install dependencies

    ```
    cd ~/go_ws/src/gitlab.com/ignitionrobotics/web/fuelserver
    ```

    Download dependencies into `vendor` folder:

        # Xenial
        $GOPATH/bin/dep ensure
        # Bionic
        ~/go_ws/bin/dep ensure

    Note: this project heavily depends on `ign-go` project. It is recommended to
    execute the following statement regularly to download the latest version of ign-go.

        # Xenial
        $GOPATH/bin/dep ensure -update gitlab.com/ignitionrobotics/web/ign-go
        # Bionic
        ~/go_ws/bin/dep ensure

1. Make the application

    ```
    cd ~/go_ws/src/gitlab.com/ignitionrobotics/web/fuelserver
    ```

    ```
    go install
    ```

1. NOTE: You should not use `go get` to get dependencies (instead use `dep ensure`). Use `go get` only when you need to modify the source code of any dependency. Alternatively, use `virtualgo` (see "Tips for local development" section below).

1. (Optional) Generate a self-signed certificate. Replace `<GO_INSTALL_PATH>` with the path to your golang installation (e.g.: `usr/local/go/`:

    ```
    cd ~/go_ws/src
    ```

    ```
    wget https://raw.githubusercontent.com/golang/go/release-branch.go1.8/src/crypto/tls/generate_cert.go
    ```

    ```
    cd ~/go_ws
    ```

    ```
    go run <GO_INSTALL_PATH>/src/generate_cert.go --host localhost --ca true
    ```

    ```
    mv cert.pem key.pem ~/go_ws/src/gitlab.com/ignitionrobotics/web/fuelserver/ssl
    ```

    ```
    export IGN_SSL_CERT=~/go_ws/src/gitlab.com/ignitionrobotics/web/fuelserver/ssl/cert.pem
    ```

    ```
    export IGN_SSL_KEY=~/go_ws/src/gitlab.com/ignitionrobotics/web/fuelserver/ssl/key.pem
    ```


    Note: to allow self certificates for localhost in Chrome, you need to put this in the chrome address bar : `chrome://flags/#allow-insecure-localhost`

1. Install mysql:

    NOTE: Install a version greater than v5.6.4. In the servers, we are currently using MySQL v5.7.21


    ```
    sudo apt-get install mysql-server
    ```

    The installer will ask you to create a root password for mysql.

1. Create the database and a user in mysql. Replace `'newuser'` with your username and `'password'` with your new password:

        # Xenial
        mysql -u root -p
        # Bionic requires sudo
        sudo mysql -u root -p

    ```
    CREATE DATABASE fuel;
    ```

    Also create a separate database to use with tests:

    ```
    CREATE DATABASE fuel_test;
    ```

    ```
    CREATE USER 'newuser'@'localhost' IDENTIFIED BY 'password';
    ```

    ```
    GRANT ALL PRIVILEGES ON fuel.* TO 'newuser'@'localhost';
    ```

    ```
    FLUSH PRIVILEGES;
    ```

    ```
    exit
    ```

1. Create a Test JWT token (this is needed for tests to pass OK -- `go test`)

    TL;DR: Just copy and paste the following env vars in your system (`.env`)

        # Test RSA256 Private key WITHOUT the -----BEGIN RSA PRIVATE KEY----- and -----END RSA PRIVATE KEY-----
        # It is used by token-generator to generate the Test JWT Token
        export TOKEN_GENERATOR_PRIVATE_RSA256_KEY=MIICWwIBAAKBgQDdlatRjRjogo3WojgGHFHYLugdUWAY9iR3fy4arWNA1KoS8kVw33cJibXr8bvwUAUparCwlvdbH6dvEOfou0/gCFQsHUfQrSDv+MuSUMAe8jzKE4qW+jK+xQU9a03GUnKHkkle+Q0pX/g6jXZ7r1/xAK5Do2kQ+X5xK9cipRgEKwIDAQABAoGAD+onAtVye4ic7VR7V50DF9bOnwRwNXrARcDhq9LWNRrRGElESYYTQ6EbatXS3MCyjjX2eMhu/aF5YhXBwkppwxg+EOmXeh+MzL7Zh284OuPbkglAaGhV9bb6/5CpuGb1esyPbYW+Ty2PC0GSZfIXkXs76jXAu9TOBvD0ybc2YlkCQQDywg2R/7t3Q2OE2+yo382CLJdrlSLVROWKwb4tb2PjhY4XAwV8d1vy0RenxTB+K5Mu57uVSTHtrMK0GAtFr833AkEA6avx20OHo61Yela/4k5kQDtjEf1N0LfI+BcWZtxsS3jDM3i1Hp0KSu5rsCPb8acJo5RO26gGVrfAsDcIXKC+bQJAZZ2XIpsitLyPpuiMOvBbzPavd4gY6Z8KWrfYzJoI/Q9FuBo6rKwl4BFoToD7WIUS+hpkagwWiz+6zLoX1dbOZwJACmH5fSSjAkLRi54PKJ8TFUeOP15h9sQzydI8zJU+upvDEKZsZc/UhT/SySDOxQ4G/523Y0sz/OZtSWcol/UMgQJALesy++GdvoIDLfJX5GBQpuFgFenRiRDabxrE9MNUZ2aPFaFp+DyAe+b4nDwuJaW2LURbr8AEZga7oQj0uYxcYw==

        # JWT Token generated by the token-generator program using the above Test RSA keys
        # This token does not expire.
        export IGN_TEST_JWT=eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0LXVzZXItaWRlbnRpdHkifQ.iV59-kBkZ86XKKsph8fxEeyxDiswY1zvPGi4977cHbbDEkMA3Y3t_zzmwU4JEmjbTeToQZ_qFNJGGNufK2guLy0SAicwjDmv-3dHDfJUH5x1vfi1fZFnmX_b8215BNbCBZU0T2a9DEFypxAQCQyiAQDE9gS8anFLHHlbcWdJdGw

        # A Test RSA256 Public key, without the -----BEGIN CERTIFICATE----- and -----END CERTIFICATE-----.
        # It is used to override the AUTH0_RSA256_PUBLIC_KEY when tests are run.
        export TEST_RSA256_PUBLIC_KEY=MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDdlatRjRjogo3WojgGHFHYLugdUWAY9iR3fy4arWNA1KoS8kVw33cJibXr8bvwUAUparCwlvdbH6dvEOfou0/gCFQsHUfQrSDv+MuSUMAe8jzKE4qW+jK+xQU9a03GUnKHkkle+Q0pX/g6jXZ7r1/xAK5Do2kQ+X5xK9cipRgEKwIDAQAB

    Long version:

    Ign Fuel assumes JWT Tokens are RSA256. In order to generate the token, follow these steps:

    1. You need to set the `TOKEN_GENERATOR_PRIVATE_RSA256_KEY` env var first. This var is a RSA256 Private key without the `-----BEGIN RSA PRIVATE KEY-----` and `-----END RSA PRIVATE KEY-----` prefix and suffix. It is used to generate Test JWT Tokens.

    1. Also please write down the corresponding `TEST_RSA256_PUBLIC_KEY` env var. This var is the RSA256 Public key (without the `-----BEGIN CERTIFICATE-----` and `-----END CERTIFICATE-----`) that will be used by the backend to validate received Test JWT tokens. NOTE: This one must be the pair of the private key `TOKEN_GENERATOR_PRIVATE_RSA256_KEY`.

    1. Generate the token:

        Build all ign-fuelserver packages and programs:

            go install gitlab.com/ignitionrobotics/web/fuelserver/...

        Note: the `...` instructs Go to build ign-fuelserver and all subpackages (eg. token-generator. You can find the extra packages in cmd/ subfolder).

            bin/token-generator

    1. The token-generator program will output a signed token. You will need to set the `IGN_TEST_JWT` env var with that generated value.

    In summary, in order to make `go test` work with JWT you will need to set the following env vars:

    * `TOKEN_GENERATOR_PRIVATE_RSA256_KEY`
    * `TEST_RSA256_PUBLIC_KEY`
    * `IGN_TEST_JWT`


1. Run the test suite

    First, make sure you have all the required env variables set:

    ```
    export IGN_DB_USERNAME=<DB username>
    ```

    ```
    export IGN_DB_PASSWORD=<DB password>
    ```

    ```
    export IGN_DB_ADDRESS=<DB IP and port. Eg: localhost:3306>
    ```

    ```
    export IGN_DB_NAME=fuel
    ```

    ```
    export IGN_DB_MAX_OPEN_CONNS=66
    ```

    ```
    export IGN_FUEL_RESOURCE_DIR=/tmp/fuel
    ```

    ```
    export IGN_POPULATE_PATH=~/.gazebo/models/
    ```

    ```
    export IGN_TEST_JWT=<JWT token>
    ```

    ```
    export TEST_RSA256_PUBLIC_KEY=< RSA256 public key without the '-----BEGIN CERTIFICATE-----' and '-----END CERTIFICATE-----'>
    Note: TEST_RSA256_PUBLIC_KEY must be able to decode and validate the IGN_TEST_JWT test token.
    ```

    Then, run all tests:
    ```
    go test gitlab.com/ignitionrobotics/web/fuelserver
    ```

1. Run the backend server

    First, make sure to set the `AUTH0_RSA256_PUBLIC_KEY` environment variable with the Auth0 RSA256 public key. This env var will be used by the backend to decode and validate any received Auth0 JWT tokens.
    Note: You can get this key from: <https://osrf.auth0.com/.well-known/jwks.json> (or from your own auth0 user). Open that url in the browser and copy the value of the `x5c` field.

    ```
    $GOPATH/bin/ign-fuelserver
    ```

1. Test in the browser

   1. If using SSL

       ```
       https://localhost:4430/1.0/models
       ```

   1. If **not** using SSL

       ```
       http://localhost:8000/1.0/models
       ```

# Permissions

Permission handling is done using the authorization library casbin

To set up the system administrator (root user), export the following
environment variable:

```
export IGN_FUEL_SYSTEM_ADMIN=sys_admin@email.org
```

Note: when running the tests, this environment variable will be overriden with
a predefined string: `root`

# Environment Variables

You may want to create an `.env.bash` file to define environment vars. Remember to add it to `.hgignore`.
Then load the env vars using `source .env.bash` from the bash terminal where you will run go commands.

## Using AWS S3 buckets

1. AWS_BUCKET_PREFIX: set it to a prefix that will be shared by all buckets
created by this server (eg. `export AWS_BUCKET_PREFIX=myFuelServer-`)
1. AWS_BUCKET_USE_IN_TESTS: set it to true if you want to us AWS S3 during tests.
Set to false to use a mock (default: false). (eg. `export AWS_BUCKET_USE_IN_TESTS=true`)
1. AWS_BUCKET_PREFIX_TEST: if AWS is enabled during tests, set this env var to a
prefix that will be shared by all buckets in tests. (eg. `export AWS_BUCKET_PREFIX_TEST=myFuelServer-tests-`)

## Flagging content and sending emails

To enable flagging of content you need to set the following env vars:

1. AWS_REGION, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY

1. IGN_FLAGS_EMAIL_FROM: The email address used as `from` when sending email notifications.

1. IGN_FLAGS_EMAIL_TO: The email address used as `to` when sending email notifications.


## Database

There are three databases in use:

  1. *ign-fuel*: a production database for use with the production Elastic Beanstalk environment,

  2. *ign-fuel-staging*: a staging database for use with the staging Elastic Beanstalk environment, and

  3. *ign-fuel-integration*: an integration database for use with the integration Elastic Beanstalk environment.

The production database should *never* be manually altered. The staging
database should match the production environment, and the purpose is to
catch migration errors on the staging Elastic beanstalk instance. The
integration database is used during testing and development. This database
is frequently wiped and altered.

### Environment variables

1. `IGN_DB_USERNAME` : Mysql user name.
1. `IGN_DB_PASSWORD` : Mysql user password.
1. `IGN_DB_ADDRESS` : Mysql address, of the form `host:port`.
1. `IGN_DB_NAME` : Mysql database name, which should be `fuel`.
1. `IGN_DB_MAX_OPEN_CONNS` : Max number of open connections in connections pool. Eg. 66.
1. `IGN_FUEL_RESOURCE_DIR` : the file system path where models will be stored.
1. `IGN_FUEL_VERBOSITY` : controls the level of output, with a default value of 2. 0 = critical messages, 1 = critical & error messages, 2 = critical & error & warning messages, 3 = critical & error & warning & informational messages, 4 = critical & error & warning & informational & debug messages
1. `AUTH0_RSA256_PUBLIC_KEY` : Auth0 RSA256 public key without the '-----BEGIN CERTIFICATE-----' and '-----END CERTIFICATE-----'
    Note: You can get this key from: <https://osrfoundation.auth0.com/.well-known/jwks.json> (or from your auth0 user). It is the "x5c" field.

## Leaderboards

There may be some cases where scores for specific organizations or circuits 
should not be displayed in competition leaderboards. There are environment 
variables available to control which organizations and circuits should not be 
displayed in leaderboards. These environment variables do not stop scores from 
being produced, they only filter `/subt/leaderboard` results.

1. `IGN_FUEL_TEST_ORGANIZATIONS` List of organizations to filter from 
leaderboard scores.
2. `IGN_FUEL_HIDE_CIRCUIT_SCORES` List of circuits to filter from leaderboard 
scores.

All of these environment variables can contain multiple comma-separated values.

## Testing and Development

1. `IGN_POPULATE_PATH` : Path to a set of Gazebo models. Setting this variable will populate the database with these models.

1. `IGN_TEST_JWT` : A JWT token used from unit tests. Note: the `TEST_RSA256_PUBLIC_KEY` must be able to decode and validate this test token.

1. `TEST_RSA256_PUBLIC_KEY` :  if present, override the Auth0 public RSA key with this test key. This one must match with the Test JWT token. It is used by unit tests (`go test`).

1. `TOKEN_GENERATOR_PRIVATE_RSA256_KEY` : RSA 256 private key Used by the Token Generator utility program to generate the Test JWT token. It must pair with the `TEST_RSA256_PUBLIC_KEY` public key.


# Naming conventions

In general we will try to follow Go naming conventions. In addition, these are own conventions:

1. For constructors / factory-methods we will use the form: `New<object>`.  See: <http://www.golangpatterns.info/object-oriented/constructors>

1. For HTTP POST handlers we use: `<object>Create`. eg: ModelCreate.
1. For HTTP DELETE handlers we use: `<object>Remove`. eg: ModelRemove.
1. An HTTP handler that returns a collection will have the form: `<object>List<format>`. eg: ModelListJSON.
    Note: `format` is optional. JSON will be assumed by default.
1. An HTTP handler that returns a single element will be: `<object>Index<format>`. eg: ModelIndexZip.
    Note: `format` is optional.  JSON will be assumed by default.
1. For HTTP OPTIONS handler we use: `<object>API`.
1. We use the func suffix `Impl` when we delegate implementation of a handler to an internal function. Eg: UserCreate handler can delegate to userCreateImpl.

# Linter

1. Get the linter

    ```
    cd ~/go_ws
    ```

    ```
    go get -u golang.org/x/lint/golint
    ```

1. Run the linter

    ```
    ./bin/golint $(go list gitlab.com/ignitionrobotics/web/fuelserver/...) | grep -v .pb.go
    ```

Note you can create this bash script:

```
#!/bin/bash
go get -u golang.org/x/lint/golint
$GOPATH/bin/golint $(go list gitlab.com/ignitionrobotics/web/fuelserver/...) | grep -v .pb.go
```


# Proto

1. If you need to modify the proto files then you will need to
run (from the proto folder):

    ```
    protoc --go_out=. *.proto
    ```

    Then update the generated import proto from code.google... to "github.com/golang/protobuf/proto"


# Coverage

1. Run test suite with coverage enabled

    ```
    go test -cover gitlab.com/ignitionrobotics/web/fuelserver
    ```

1. Tip. Add this function to your ~/.bashrc

```
cover () {
  t="/tmp/go-cover.$$.tmp"
  go test -covermode=count -coverprofile=$t $@ && go tool cover -html=$t && unl$
}
```

Then run the function from your project folder. Tests will be run and a browser window will open with
coverage results.


# Integration deployment

If it's the first time that you deploy on `integration`:

1. [Install eb CLI tool](http://docs.aws.amazon.com/elasticbeanstalk/latest/dg/eb-cli3-install.html) (if needed).

1. Configure eb:

    ```
    eb init
    ```

    And choose the following options:

    1. Select `us-east-1` as region.

    1. Select `ign-fuel-server` as the application name.

    1. Select `ign-fuel-server-integration` as the environment name.

Otherwise, just type:

    eb deploy


# Staging/Production deployment

The `staging` and `production` branches push code through bitbucket
pipelines to AWS Elastic Beanstalk (EBS).

1. Push code to staging. Make sure pipelines completes successfully.

1. Test `staging.api.ignitionfuel.org`

1. When ready, Swap Environment URLs with the production EBS environment.

1. Merge the `staging` branch into the `production` branch, and push.

1. Test `staging.api.ignitionfuel.org` again

1. Swap the EBS environment URLs back.

# AWS Configuration

* Elastic Beanstalk runs go in a docker container

* AWS Relation Database (RDS) runs an instance of mysql.

* Each EC2 instance started by EBS mounts an NFS filesystem, via Elastic
Filesystem, on `/fuel`. This filesystem store all the mercurial
repositories.

## AWS RDS

There are two mysql databases hosted on Amazon.

1. `ign-fuel`: The production database.

    * Endpoint: ign-fuel.cpznmiopbczj.us-east-1.rds.amazonaws.com:3306

1. `ign-fuel-dev`: The development and testing database. You can write tests that will run on bitbucket pipelines against this database. Make sure to clean the database up after each test.

    * Endpoint: ign-fuel-dev.cpznmiopbczj.us-east-1.rds.amazonaws.com:3306

# Development

See the Database section above for information on the different database
options.

## Transactions

Try to create and commit transactions within main Handlers, and not in the helper functions.

## REST Documentation

Swagger is used to document the REST API. This includes both model and
route information.

**Do not manually edit `swagger.json`**

**Process**

1. Document a route or model following [this documentation](https://goswagger.io/generate/spec.html).

1. Install swagger inside your `GOPATH`

    * go get -u github.com/go-swagger/go-swagger/cmd/swagger
    * go install github.com/go-swagger/go-swagger/cmd/swagger

1. Generate the `swagger.json` file. This file will be used by a webserver
   to display the API documentation.

    ```
    ./bin/swagger generate spec -o ./src/gitlab.com/ignitionrobotics/web/fuelserver/swagger.json -b ./src/gitlab.com/ignitionrobotics/web/fuelserver/ -m
    ```

1. Commit and push your changes to the repository.

1. View the results at [http://doc.ignitionfuel.org](http://doc.ignitionfuel.org). Enter
   a new `swagger.json` in the `Explore` box to see a different version of
   the API.

**Useful links**

1. [Swagger json documentation](https://goswagger.io/generate/spec.html)

  * This page documents how to write swagger documentation that will be
  parsed to generate the swagger.json file.

1. [Our S3 swagger website](http://doc.ignitionfuel.org)

  * This is an instance of [swagger-ui](https://github.com/swagger-api/swagger-ui/tree/master/dist), where the `index.html` file was edited to point to our swagger file.

## Log Files

Two web services are used for log management.

1. rollbar.com

Rollbar aggregates log messages from the application. Application log
messages are sent to rollbar when a REST error is returned to a client or by
using one of `ign.Debug`, `ign.Info`, `ign.Warning`, `ign.Error`, or
`ign.Critical`.

Messages are sent to rollbar asynchronously. This could result in messages
appearing out of order on rollbar's UI. In addition to the timestamp
generated by rollbar upon message arrival, rollbar keeps
a `metadata.customer_timestamp` which should be the application's timestamp.
You can use the RQL console with `metadata.custom_timestamp` to reorder
items. Steps:

    1. Click on `RQL` in rollbar's top toolbar.
    2. Enter an SQL query like the following:

        ```
        SELECT *
        FROM item_occurrence
        WHERE item.counter = 932
        ORDER BY metadata.customer_timestamp DESC
        LIMIT 0, 20
        ```
    3. Click 'Run'


2. papertrail.com

Papertrail aggregates system log messages. Log file upload to Papertrail
happens automatically. Configuration, including specification of system log
files to monitor, is handled in
`.ebextensions/remote_syslog.ebextensions.config`.

## Debugging inside docker container

If you ever need to debug the application as if it were running in AWS or the pipelines, you need to do it from inside its docker containter.
To do that:

Most ideas taken from here:
Mysql and Docker https://docs.docker.com/samples/library/mysql/#-via-docker-stack-deploy-or-docker-compose

1. First create the docker image for the ign-fuelserver. `docker build ign-fuelserver` . Write down its image ID.

1. Then run a dockerized mysql database. `docker run --name my-mysql -e MYSQL_ROOT_PASSWORD=<desired-root-pwd> -d mysql:5.7.21`
This will create a mysql docker container with an empty mysql in it.

1. Then you need to connect to that mysql container instance to run commands: `docker exec -it my-mysql bash`. From inside the container, connect to mysql using the client (eg. `mysql -u root -p`) and create databases fuel and fuel_test. eg: `create database fuel_test;`.

1. Run the ign-fuelserver docker and link it to mysql database. `docker run --name ign-fuelserver --rm --link my-mysql:mysql -ti <fuelserver-image-id> /bin/bash`

1. Then from inside the server container you need to set the Env Var that points to the linked docker mysql. eg. `export IGN_DB_ADDRESS="172.17.0.2:3306"`

After that you can source your env vars and run commands such as `go test`.

## Tips for local development using multiple dependent projects

This is useful when you need to test uncommitted changes from a project depedency. Eg. when you need to test changes in ign-go using ign-fuelserver.

Install vg (`virtualgo`) . This tool is used on top of `go dep`: https://github.com/GetStream/vg

First, initialize vg support in your system. In a terminal, run:

- `export PATH=$PATH:$GOPATH/bin && eval "$(vg eval --shell bash)"`

Tip: I have pushed a .vgenable script that can be `sourced` later to enable vg support in current terminal. Eg. `source .vgenable`

How to use vg:

1. `vg init` (only the first time, to initialize the project with vg)

1. `vg ensure` (this command delegates to `dep ensure` and then removes the `vendor` folder)

1. To add or update dependencies (into Gopck.toml) use: `vg ensure -- -update <dependency>` (or just use normal `dep ensure -update <dependency>` style, and later run `vg ensure` to move dependencies into vg's workspace)

1. How to switch to a local version of a dependency: eg. `vg localInstall gitlab.com/ignitionrobotics/web/ign-go`