# ElasticSearch Guide

## Install ElasticSearch and Kibana

This section describes how to setup ElasticSearch and Kibana locally for development and testing.

### Ubuntu

1. sudo apt install openjdk-8-jdk
2. wget -qO - https://artifacts.elastic.co/GPG-KEY-elasticsearch | sudo apt-key add -
3. sudo sh -c 'echo "deb https://artifacts.elastic.co/packages/7.x/apt stable main" > /etc/apt/sources.list.d/elastic-7.x.list'
4. sudo apt update
5. sudo apt install elasticsearch
6. sudo systemctl start elasticsearch
7. sudo systemctl enable elasticsearch

8. Test using `curl -X GET "localhost:9200"`, which should return

```
{
  "name" : "ElasticSearch",
  "cluster_name" : "elasticsearch",
  "cluster_uuid" : "SMYhVWRiTwS1dF0pQ-h7SQ",
  "version" : {
    "number" : "7.6.1",
    "build_flavor" : "default",
    "build_type" : "deb",
    "build_hash" : "aa751e09be0a5072e8570670309b1f12348f023b",
    "build_date" : "2020-02-29T00:15:25.529771Z",
    "build_snapshot" : false,
    "lucene_version" : "8.4.0",
    "minimum_wire_compatibility_version" : "6.8.0",
    "minimum_index_compatibility_version" : "6.0.0-beta1"
  },
  "tagline" : "You Know, for Search"
}
```
9. sudo apt install kibana
10. sudo systemctl enable kibana
11. sudo systemctl start kibana
12. Test by open a browser page to http://localhost:5601/status

## Configure Fuel

Fuel maintains an `elastic_search_configs` SQL table that lists available
ElasticSearch servers. There should be only one entry in this table where
`is_primary` is true. If multiple happen to marked as true, then Fuel will
use the first.

Fuel will connect to an ElasticSearch server on start, if one is available.

Configuration of ElasticSearch can be done at run time through the
`/admin/search` PATCH route. See documentation for the
AdminElasticSearchHandler function.

If you have just installed ElasticSearch locally, then you can use the
following commands to connect to the server and populate the server with the
contents of your SQL database. Make sure you are a system admin by adding
your username to the `IGN_FUEL_SYSTEM_ADMIN` environment variable.

1. `curl -k -H "Content-Type: application/json" -X POST http://localhost:8000/1.0/admin/search -d '{"address":"http://localhost:9200", "primary":true}' --header "Private-token: YOUR_TOKEN"`

2. `curl -k -X GET http://localhost:8000/1.0/admin/search/reconnect --header "Private-token: YOUR_TOKEN"`

3. `curl -k -X GET http://localhost:8000/1.0/admin/search/rebuild --header "Private-token: YOUR_TOKEN"`

Check your configurations by getting the list of ElasticSearch configs.

```
curl -k -X GET http://localhost:8000/1.0/admin/search --header "Private-token: YOUR_TOKEN"
```

## Searching

You can perform searches on models and worlds. We'll focus on models.

1. General search: `/models?q=<search_term>`. For example:

```
http://localhost:8000/1.0/models?q=robot
```

2. Search with capture: `/models?q=<search_term>*`. For example

```
http://localhost:8000/1.0/models?q=rob*
```

3. Field search: `/models?q=<field_name>:<search_term>`. For example, to search for a name:

```
http://localhost:8000/1.0/models?q=name:pioneer
```

4. Metadata search:
   `/modesl?q=metadata.key:<search_term>%26metadata.value:<search_term>`.
   You can specify a metdata.key and/or metdata.value.
   For example:

```
http://localhost:8000/1.0/models?q=metadata.key:robot%26metadata.value:red
```

5. You can combine the above by separating each part with a `%26`.

## Appendix

#### Links
* https://github.com/elastic/go-elasticsearch
* https://github.com/elastic/go-elasticsearch/tree/master/_examples
* https://medium.com/@ashish_fagna/getting-started-with-elasticsearch-creating-indices-inserting-values-and-retrieving-data-e3122e9b12c6

#### ElasticSearch Definitions

1. Index: This is equivalent to a database in a relational DB.
2. Type: This is equivalent to a table in a relational DB.
3. Document: This is equivalent to a row in a relational DB. 

#### Elastic REST API

* REST API Format : `http://host:port/[index]/[type]/[_action/id]`

* To get get all documents: `http://localhost:9200/fuel/_search?pretty=true&q=*:*`
