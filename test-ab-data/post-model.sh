#!/bin/bash
#ab -v 2 -k -c $AB_C -n $AB_N -T "multipart/form-data; boundary=---------------------------103633748619302808251276201494" -p post-model-data.txt -H "Authorization: Bearer ${AB_AUTH_HEADER}" $AB_SERVER/1.0/models\
hey -c $AB_C -n $AB_N -T "multipart/form-data; boundary=---------------------------103633748619302808251276201494" -m POST -D post-model-data.txt -H "Authorization: Bearer ${AB_AUTH_HEADER}" $AB_SERVER/1.0/models
# expected result: after running this, the database should contain just one model.
