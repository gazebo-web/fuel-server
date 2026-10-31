# This folder contains files to be used when testing with with sqlmap.

NOTE: before running POST requests make sure to create a user with name "test-username".

Also use the TEST token as authorization token in requests. The commands below
read it from `IGN_TEST_JWT` (see `scripts/test-keys.sh`).

We need to first manually create a user and a model to test some urls.

# First, create user (using TEST AUTH0_RSA256_PUBLIC_KEY)
```
$ curl -k -H "Content-Type: application/json" -X POST -d '{"name":"John Doe", "username":"test-username", "email":"johndoe@example.com", "org":"my org"}' http://localhost:8000/1.0/users --header "authorization: Bearer $IGN_TEST_JWT"
{"name":"John Doe","username":"test-username","email":"johndoe@example.com","org":"my org","id":3}
```

# Then a model 
```
$ touch /tmp/empty-model-file.txt
```

```
$ curl -k -X POST -F license=1 -F modelName=testModel -F description=testDesc -F permission=0 -F owner=test-username -F file=@/tmp/empty-model-file.txt http://localhost:8000/1.0/models --header "authorization: Bearer $IGN_TEST_JWT"
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

`model-post-data.txt` has an `<IGN_TEST_JWT>` placeholder. Replace it with the
token first:

```
sed "s/<IGN_TEST_JWT>/$IGN_TEST_JWT/" test-sqlmap/model-post-data.txt > /tmp/model-post-data.txt
```

```
python sqlmap.py -v 2 -r /tmp/model-post-data.txt --random-agent --delay=1 --timeout=15 --retries=2 --keep-alive --threads=5 --batch --dbms=MySQL --os=Linux -a --level=3 --is-dba --dbs --tables --technique=BEUST -s /tmp/scan_report.txt --flush-session -t /tmp/scan_trace.txt --fresh-queries
```
