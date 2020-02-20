#!/bin/bash

#cat get-urls.txt | envsubst | parallel "ab -k -c $AB_C -n $AB_N -H \"Authorization: Bearer ${AB_AUTH_HEADER}\" {}"
cat get-urls.txt | envsubst | parallel "hey -c $AB_C -n $AB_N -H \"Authorization: Bearer ${AB_AUTH_HEADER}\" {}"
