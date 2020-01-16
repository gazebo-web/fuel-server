#!/bin/bash
# This script needs 'hey'. To install it: go get -u github.com/rakyll/hey
export PATH=$PATH:$GOPATH/bin
bash post-user.sh & bash post-model.sh & bash parallel-gets.sh

# to see mysql open conns, execute this query:s
# show status where `variable_name` = 'Threads_connected';
