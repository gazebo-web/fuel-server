#!/bin/bash

# This script assumes you have curl, git and python installed
# and the server running on http://localhost:8000.
# It assumes there is NO user created with the TEST Auth public key:
# --header 'authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0LXVzZXItaWRlbnRpdHkifQ.iV59-kBkZ86XKKsph8fxEeyxDiswY1zvPGi4977cHbbDEkMA3Y3t_zzmwU4JEmjbTeToQZ_qFNJGGNufK2guLy0SAicwjDmv-3dHDfJUH5x1vfi1fZFnmX_b8215BNbCBZU0T2a9DEFypxAQCQyiAQDE9gS8anFLHHlbcWdJdGw'
# You can replace it with your own.

git clone https://github.com/sqlmapproject/sqlmap.git /tmp/sqlmap-dev

# First, create user (using TEST AUTH0_RSA256_PUBLIC_KEY)
# Note there should be no folder /tmp/fuel/anonymous for this to work
curl -k -H "Content-Type: application/json" -X POST -d '{"name":"John Doe", "username":"test-username", "email":"johndoe@example.com", "org":"my org"}' http://localhost:8000/1.0/users --header 'authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0LXVzZXItaWRlbnRpdHkifQ.iV59-kBkZ86XKKsph8fxEeyxDiswY1zvPGi4977cHbbDEkMA3Y3t_zzmwU4JEmjbTeToQZ_qFNJGGNufK2guLy0SAicwjDmv-3dHDfJUH5x1vfi1fZFnmX_b8215BNbCBZU0T2a9DEFypxAQCQyiAQDE9gS8anFLHHlbcWdJdGw'

# Then a model 
touch /tmp/empty-model-file.txt
curl -k -X POST -F license=1 -F modelName=testModel -F description=testDesc -F permission=0 -F owner=test-username -F file=@/tmp/empty-model-file.txt http://localhost:8000/1.0/models --header 'authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0LXVzZXItaWRlbnRpdHkifQ.iV59-kBkZ86XKKsph8fxEeyxDiswY1zvPGi4977cHbbDEkMA3Y3t_zzmwU4JEmjbTeToQZ_qFNJGGNufK2guLy0SAicwjDmv-3dHDfJUH5x1vfi1fZFnmX_b8215BNbCBZU0T2a9DEFypxAQCQyiAQDE9gS8anFLHHlbcWdJdGw'

# Run bulk of GET tests
python /tmp/sqlmap-dev/sqlmap.py -v 2 -m $GOPATH/src/bitbucket.org/ignitionrobotics/ign-fuelserver/test-sqlmap/urls.txt --random-agent --delay=1 --timeout=15 --retries=2 --keep-alive --threads=5 --batch --dbms=MySQL --os=Linux -a --level=3 --is-dba --dbs --tables --technique=BEUST -s /tmp/scan_report.txt --flush-session -t /tmp/scan_trace.txt --fresh-queries

# Now test POST model request
python /tmp/sqlmap-dev/sqlmap.py -v 2 -r $GOPATH/src/bitbucket.org/ignitionrobotics/ign-fuelserver/test-sqlmap/model-post-data.txt --random-agent --delay=1 --timeout=15 --retries=2 --keep-alive --threads=5 --batch --dbms=MySQL --os=Linux -a --level=3 --is-dba --dbs --tables --technique=BEUST -s /tmp/scan_report.txt --flush-session -t /tmp/scan_trace.txt --fresh-queries 
