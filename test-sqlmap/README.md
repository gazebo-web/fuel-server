# This folder contains files to be used when testing with with sqlmap.

NOTE: before running POST requests make sure to create a user with name "test-username".

Also use the TEST token as authorization token in requests.

We need to first manually create a user and a model to test some urls.

# First, create user (using TEST AUTH0_RSA256_PUBLIC_KEY)
```
$ curl -k -H "Content-Type: application/json" -X POST -d '{"name":"John Doe", "username":"test-username", "email":"johndoe@example.com", "org":"my org"}' http://localhost:8000/1.0/users --header 'authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0LXVzZXItaWRlbnRpdHkifQ.iV59-kBkZ86XKKsph8fxEeyxDiswY1zvPGi4977cHbbDEkMA3Y3t_zzmwU4JEmjbTeToQZ_qFNJGGNufK2guLy0SAicwjDmv-3dHDfJUH5x1vfi1fZFnmX_b8215BNbCBZU0T2a9DEFypxAQCQyiAQDE9gS8anFLHHlbcWdJdGw'
{"name":"John Doe","username":"test-username","email":"johndoe@example.com","org":"my org","id":3}
```

# Then a model 
```
$ touch /tmp/empty-model-file.txt
```

```
$ curl -k -X POST -F license=1 -F modelName=testModel -F description=testDesc -F permission=0 -F owner=test-username -F file=@/tmp/empty-model-file.txt http://localhost:8000/1.0/models --header 'authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0LXVzZXItaWRlbnRpdHkifQ.iV59-kBkZ86XKKsph8fxEeyxDiswY1zvPGi4977cHbbDEkMA3Y3t_zzmwU4JEmjbTeToQZ_qFNJGGNufK2guLy0SAicwjDmv-3dHDfJUH5x1vfi1fZFnmX_b8215BNbCBZU0T2a9DEFypxAQCQyiAQDE9gS8anFLHHlbcWdJdGw'
```

# Then you can execute sqlmap commads like

```
python sqlmap.py -v 2 -u "http://localhost:8000/1.0/models/45bb9cb9-e83c-4415-b5e7-d464c76d6521*" --random-agent --delay=1 --timeout=15 --retries=2 --keep-alive --threads=5 --batch --dbms=MySQL --os=Linux -a --level=3 --is-dba --dbs --tables --technique=BEUST -s /tmp/scan_report.txt --flush-session -t /tmp/scan_trace.txt --fresh-queries
```

# Or bulk test urls

```
python sqlmap.py -v 2 -m test-sqlmap/urls.txt --random-agent --delay=1 --timeout=15 --retries=2 --keep-alive --threads=5 --batch --dbms=MySQL --os=Linux -a --level=3 --is-dba --dbs --tables --technique=BEUST -s /tmp/scan_report.txt --flush-session -t /tmp/scan_trace.txt --fresh-queries
```

# Or POST requests from files

```
python sqlmap.py -v 2 -r test-sqlmap/model-post-data.txt --random-agent --delay=1 --timeout=15 --retries=2 --keep-alive --threads=5 --batch --dbms=MySQL --os=Linux -a --level=3 --is-dba --dbs --tables --technique=BEUST -s /tmp/scan_report.txt --flush-session -t /tmp/scan_trace.txt --fresh-queries
```
