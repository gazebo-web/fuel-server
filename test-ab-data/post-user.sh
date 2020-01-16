#!/bin/bash

#ab -v 2 -k -c $AB_C -n $AB_N -T application/json -p post-user-data.txt -H "Authorization: Bearer ${AB_AUTH_HEADER}" $AB_SERVER/1.0/users
hey -c $AB_C -n $AB_N -T application/json -m POST -D post-user-data.txt -H "Authorization: Bearer ${AB_AUTH_HEADER}" $AB_SERVER/1.0/users
# expected result: after running this, the database should contain just one user.
