package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/gazebo-web/fuel-server/bundles/models"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/bundles/worlds"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/permissions"
	"github.com/gazebo-web/fuel-server/proto"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ElasticSearch indices
var fuelIndices = []string{"fuel_models", "fuel_worlds"}

// ElasticSearchConfig is a configuration for an ElasticSearch server.
// swagger:model
type ElasticSearchConfig struct {
	// ID is the primary key
	ID uint `gorm:"primary_key" json:"id"`
	// CreatedAt is the time the entry was created.
	CreatedAt time.Time `gorm:"type:timestamp(3) NULL"`
	// UpdatedAt is the time the entry was update.
	UpdatedAt time.Time
	// Added 2 milliseconds to DeletedAt field, and added it to the unique index to help disambiguate
	// when soft deleted rows are involved.
	DeletedAt *time.Time `gorm:"type:timestamp(2) NULL; unique_index:idx_modelname_owner" sql:"index"`

	// Address of the server. This must contain either "http" or "https".
	Address string `json:"address"`

	// Username for basic authentication. Optional.
	Username string `json:"username"`

	// Password for basic authentication. Optional.
	Password string `json:"password"`

	// True if this is the server to use by default.
	IsPrimary bool `json:"primary"`
}

// ElasticSearchConfigs is a list of ElasticSearchConfig
// swagger:model
type ElasticSearchConfigs []ElasticSearchConfig

// AdminSearchRequest is a request to alter the ElasticSearchConfig
// swagger:model
type AdminSearchRequest struct {
	// Address of the server. This must contain either "http" or "https".
	Address string `json:"address"`

	// Username for basic authentication. Optional.
	Username string `json:"username"`

	// Password for basic authentication. Optional.
	Password string `json:"password"`

	// True if this is the server to use by default.
	Primary bool `json:"primary"`
}

// AdminSearchResponse contains a response to an AdminSearchRequest.
// swagger:model
type AdminSearchResponse struct {
	Message string `json:"status"`
}

// DeleteElasticSearchHandler deletes an elasticsearch config
//
// curl -k -X DELETE http://localhost:8000/1.0/admin/search/{config_id} --header "Private-token: YOUR_TOKEN"
func DeleteElasticSearchHandler(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	user, ok, errMsg := getUserFromJWT(tx, r)

	if !ok && (errMsg.ErrCode != gz.ErrorAuthJWTInvalid &&
		errMsg.ErrCode != gz.ErrorAuthNoUser) {
		return nil, &errMsg
	}

	if !globals.Permissions.IsSystemAdmin(*user.Username) {
		return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
	}

	// Get the config id
	configID, valid := mux.Vars(r)["config_id"]
	if !valid {
		return nil, gz.NewErrorMessage(gz.ErrorIDNotInRequest)
	}

	var config ElasticSearchConfig

	// Find the config
	if err := tx.First(&config, configID).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorIDNotFound, err)
	}

	// Try to delete the config.
	if err := tx.Delete(&config).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbDelete, err)
	}

	// Return the config that was deleted.
	return config, nil
}

// ModifyElasticSearchHandler modifies an existing config
//
// curl -k -H "Content-Type: application/json" -X PATCH http://localhost:8000/1.0/admin/search/{config_id} -d '{"address":"http://localhost:9200", "primary":true, "username":"my_username", "password":"my_password"}' --header "Private-token: YOUR_TOKEN"
func ModifyElasticSearchHandler(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	user, ok, errMsg := getUserFromJWT(tx, r)

	if !ok && (errMsg.ErrCode != gz.ErrorAuthJWTInvalid &&
		errMsg.ErrCode != gz.ErrorAuthNoUser) {
		return nil, &errMsg
	}

	if !globals.Permissions.IsSystemAdmin(*user.Username) {
		return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
	}

	// Get the config id
	configID, valid := mux.Vars(r)["config_id"]
	if !valid {
		return nil, gz.NewErrorMessage(gz.ErrorIDNotInRequest)
	}

	// Parse the request
	var request AdminSearchRequest
	if em := ParseStruct(&request, r, false); em != nil {
		return nil, em
	}

	var dbConfig ElasticSearchConfig

	// Find the config
	if err := tx.First(&dbConfig, configID).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorIDNotFound, err)
	}

	dbConfig.Address = request.Address
	dbConfig.Username = request.Username
	dbConfig.Password = request.Password
	dbConfig.IsPrimary = request.Primary

	if err := tx.Save(&dbConfig).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}

	// If new primary, then make other entries not be primary.
	if request.Primary {
		tx.Model(ElasticSearchConfig{}).Where("is_primary = 1 and address != ?", request.Address).Select("is_primary").Updates(map[string]interface{}{"is_primary": "0"})
	}

	return dbConfig, nil
}

// CreateElasticSearchHandler creates a new elastic search config
//
// curl -k -H "Content-Type: application/json" -X POST http://localhost:8000/1.0/admin/search -d '{"address":"http://localhost:9200", "primary":true}' --header "Private-token: YOUR_TOKEN"
func CreateElasticSearchHandler(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	user, ok, errMsg := getUserFromJWT(tx, r)

	if !ok && (errMsg.ErrCode != gz.ErrorAuthJWTInvalid &&
		errMsg.ErrCode != gz.ErrorAuthNoUser) {
		return nil, &errMsg
	}

	if !globals.Permissions.IsSystemAdmin(*user.Username) {
		return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
	}

	// Parse the request
	var request AdminSearchRequest
	if em := ParseStruct(&request, r, false); em != nil {
		return nil, em
	}

	dbConfig := ElasticSearchConfig{
		Address:   request.Address,
		Username:  request.Username,
		Password:  request.Password,
		IsPrimary: request.Primary,
	}

	if err := tx.Create(&dbConfig).Error; err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorDbSave, err)
	}

	// If new primary, then make other not primary.
	if request.Primary {
		tx.Model(ElasticSearchConfig{}).Where("is_primary = 1 and address != ?", request.Address).Select("is_primary").Updates(map[string]interface{}{"is_primary": "0"})
	}

	return dbConfig, nil
}

// ListElasticSearchHandler returns a list of the elastic search configs
//
// curl -k -X GET http://localhost:8000/1.0/admin/search --header "Private-token: YOUR_TOKEN"
func ListElasticSearchHandler(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	user, ok, errMsg := getUserFromJWT(tx, r)

	if !ok && (errMsg.ErrCode != gz.ErrorAuthJWTInvalid &&
		errMsg.ErrCode != gz.ErrorAuthNoUser) {
		return nil, &errMsg
	}

	if !globals.Permissions.IsSystemAdmin(*user.Username) {
		return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
	}

	var dbConfigs ElasticSearchConfigs

	tx.Find(&dbConfigs)

	return dbConfigs, nil
}

// ReconnectElasticSearchHandler reconnects to the primary ElasticSearch config
//
// curl -k -X GET http://localhost:8000/1.0/admin/search/reconnect --header "Private-token: YOUR_TOKEN"
func ReconnectElasticSearchHandler(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	user, ok, errMsg := getUserFromJWT(tx, r)

	if !ok && (errMsg.ErrCode != gz.ErrorAuthJWTInvalid &&
		errMsg.ErrCode != gz.ErrorAuthNoUser) {
		return nil, &errMsg
	}

	if !globals.Permissions.IsSystemAdmin(*user.Username) {
		return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
	}

	if err := connectToElasticSearch(r.Context()); err != nil {
		return nil, gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	response := AdminSearchResponse{Message: "Reconnected"}
	return response, nil
}

// RebuildElasticSearchHandler rebuilds the indices for the primary config
//
// curl -k -X GET http://localhost:8000/1.0/admin/search/rebuild --header "Private-token: YOUR_TOKEN"
func RebuildElasticSearchHandler(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	user, ok, errMsg := getUserFromJWT(tx, r)

	if !ok && (errMsg.ErrCode != gz.ErrorAuthJWTInvalid &&
		errMsg.ErrCode != gz.ErrorAuthNoUser) {
		return nil, &errMsg
	}

	if !globals.Permissions.IsSystemAdmin(*user.Username) {
		return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
	}

	deleteIndices(r.Context())
	createIndices(r.Context())
	models.ElasticSearchUpdateAll(r.Context(), tx)
	worlds.ElasticSearchUpdateAll(r.Context(), tx)

	response := AdminSearchResponse{Message: "Rebuilt indices"}

	return response, nil
}

// UpdateElasticSearchHandler updates the primay ElasticSearch.
//
// curl -k -X GET http://localhost:8000/1.0/admin/search/update --header "Private-token: YOUR_TOKEN"
func UpdateElasticSearchHandler(tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.ErrMsg) {

	user, ok, errMsg := getUserFromJWT(tx, r)

	if !ok && (errMsg.ErrCode != gz.ErrorAuthJWTInvalid &&
		errMsg.ErrCode != gz.ErrorAuthNoUser) {
		return nil, &errMsg
	}

	if !globals.Permissions.IsSystemAdmin(*user.Username) {
		return nil, gz.NewErrorMessage(gz.ErrorUnauthorized)
	}

	models.ElasticSearchUpdateAll(r.Context(), tx)
	worlds.ElasticSearchUpdateAll(r.Context(), tx)

	response := AdminSearchResponse{Message: "Updated indices"}

	return response, nil
}

// connectToElasticSearch Establishes a connection to elastic search
func connectToElasticSearch(ctx context.Context) error {
	var err error
	var response map[string]interface{}

	var dbConfig ElasticSearchConfig

	// Get the first primary configuration
	if err = globals.Server.Db.Where("is_primary = 1").First(&dbConfig).Error; err != nil {
		gz.LoggerFromContext(ctx).Debug("No ElasticSearch configuration, skipping")
		return err
	}

	cfg := elasticsearch.Config{
		Addresses: []string{dbConfig.Address},
		Username:  dbConfig.Username,
		Password:  dbConfig.Password,
	}

	// Create a new elastic search client.
	globals.ElasticSearch, err = elasticsearch.NewClient(cfg)
	if err != nil {
		gz.LoggerFromContext(ctx).Error("Elastic search error creating new elasticsearch client:", err)
		return err
	}

	// Get cluster info
	res, err := globals.ElasticSearch.Info()
	if err != nil {
		gz.LoggerFromContext(ctx).Error("Elastic search error getting response:", err)
		return err
	}
	defer res.Body.Close()

	// Check response status
	if res.IsError() {
		gz.LoggerFromContext(ctx).Error("Elastic search error:", res.String())
	}

	// Deserialize the response into a map.
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		gz.LoggerFromContext(ctx).Error("Error parsing the response body:", err)
	}

	// Print client and server version numbers.
	gz.LoggerFromContext(ctx).Info("Elastic Search Client:", elasticsearch.Version)
	gz.LoggerFromContext(ctx).Info("Elastic Search Server:",
		response["version"].(map[string]interface{})["number"])

	return nil
}

// deleteIndices delets the elasticsearch indices.
func deleteIndices(ctx context.Context) {
	// Set up the request object.
	deleteReq := esapi.IndicesDeleteRequest{
		Index: fuelIndices,
	}

	// Perform the request with the client.
	_, err := deleteReq.Do(context.Background(), globals.ElasticSearch)
	if err != nil {
		gz.LoggerFromContext(ctx).Error("Error delete indices with response:", err)
	}
}

// createFuelMappings Creates the an index and appropriate mappings.
// It's important to set "type":"nested" for nested documents such as "metadata"
// and "tags", otherwise nested queries will fail.
func createIndex(ctx context.Context, indexName string) {

	if globals.ElasticSearch == nil {
		return
	}

	// The set of mappings for the Fuel index
	var mappings = `{
    "mappings": {
      "properties": {
        "categories": {
          "type": "text",
          "fields": {
            "keyword": {
              "type": "keyword",
              "ignore_above": 256
            }
          }
        },
        "creator": {
          "type": "text",
          "fields": {
            "keyword": {
              "type": "keyword",
              "ignore_above": 256
            }
          }
        },
        "description": {
          "type": "text",
          "fields": {
            "keyword": {
              "type": "keyword",
              "ignore_above": 256
            }
          }
        },
        "license": {
          "type": "text",
          "fields": {
            "keyword": {
              "type": "keyword",
              "ignore_above": 256
            }
          }
        },
        "metadata": {
          "type": "nested",
          "properties": {
            "key": {
              "type": "text",
              "fields": {
                "keyword": {
                  "type": "keyword",
                  "ignore_above": 256
                }
              }
            },
            "value": {
              "type": "text",
              "fields": {
                "keyword": {
                  "type": "keyword",
                  "ignore_above": 256
                }
              }
            }
          }
        },
        "name": {
          "type": "text",
          "fields": {
            "keyword": {
              "type": "keyword",
              "ignore_above": 256
            }
          }
        },
        "owner": {
          "type": "text",
          "fields": {
            "keyword": {
              "type": "keyword",
              "ignore_above": 256
            }
          }
        },
        "tags": {
          "type": "text",
          "fields": {
            "keyword": {
              "type": "keyword",
              "ignore_above": 256
            }
          }
        },
        "collections": {
          "type": "text",
          "fields": {
            "keyword": {
              "type": "keyword",
              "ignore_above": 256
            }
          }
        }
      }
    }
  }`

	// Set up the request object.
	mappingReq := esapi.IndicesCreateRequest{
		Index: indexName,
		Body:  strings.NewReader(mappings),
	}

	// Perform the request with the client.
	res, err := mappingReq.Do(context.Background(), globals.ElasticSearch)
	if err != nil {
		gz.LoggerFromContext(ctx).Error("Error creating the index with response:", err)
	}
	defer res.Body.Close()

	// Deserialize the response into a map.
	var response map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		gz.LoggerFromContext(ctx).Error("Error parsing the response body:", err)
	}

	gz.LoggerFromContext(ctx).Info("Created fuel elastic search index and mappings.")
}

// createIndices will create the fuel indices and mappings.
func createIndices(ctx context.Context) {
	for _, index := range fuelIndices {
		// Check if the "fuel" index exists.
		indexExistsReq := esapi.IndicesExistsRequest{
			Index: []string{index},
		}

		// Perform the request with the client.
		res, err := indexExistsReq.Do(context.Background(), globals.ElasticSearch)
		if err != nil {
			gz.LoggerFromContext(ctx).Error("Error getting the indices with response:", err)
		}

		// If the status code is not 200, then we need to create the index,
		// mappings.
		if res.StatusCode != 200 {
			// Create the fuel index and mappings
			createIndex(ctx, index)
		}
	}
}

// elasticSearch performs a search
// It's recommended that we don't use ElasticSearch for empty searches.
// Instead, use a direct SQL select.
func elasticSearch(index string, pr *gz.PaginationRequest, owner *string, order, search string, user *users.User, tx *gorm.DB, w http.ResponseWriter, r *http.Request) (interface{}, *gz.PaginationResult, *gz.ErrMsg) {

	// Debug
	//fmt.Printf("=== Start of ElasticSearch[%s] ===\n", index)
	//fmt.Printf("* Raw search string[%s]\n", search)

	// Build search request body.
	var buf bytes.Buffer
	var query map[string]interface{}

	ctx := r.Context()

	// Did the user specify a search, or is it empty (`?q=`)?
	// It's recommended that we don't use ElasticSearch for empty searches.
	// Instead, use a direct SQL select.
	// Keeping this check here just in case.
	if len(search) > 0 {

		// The "must" variable will hold each portion of the boolean query.
		// See: https://www.elastic.co/guide/en/elasticsearch/reference/current/query-dsl-bool-query.html
		var must []interface{}

		// metaDatumQuery contains a key/value search pair.
		type metaDatumQuery struct {
			Key   *string
			Value *string
		}

		// metadata contains a list of metadata search terms.
		var metadata []metaDatumQuery

		// Split the search string into its component terms.
		// A search string may contain one or more search terms separated
		// by ampersands.
		terms := strings.Split(search, "&")
		for _, term := range terms {
			// Debug
			//fmt.Printf("* Term[%s]\n", term)

			// Get each part of the search term.
			// A search term may have two parts separated by a colon.
			//
			// If a colon is present, then the first part is a field name and
			// the second part is the search to perform on the field.
			//
			// If a colon is *not* present then the part is the search to
			// perform.
			parts := strings.Split(term, ":")

			// Debug
			// for i, part := range parts {
			//   fmt.Printf("  Part %d=%s\n", i, part)
			// }

			// This will hold the "match" portion of an Elastic Search query.
			var match map[string]interface{}

			// If there are multiple parts in a search term, then it is understood
			// that the first part of the search term is a field to search within
			// and the second part is the query.
			//
			// Metadata search is a special case since the user could, optionally,
			// specify both a key and value in their search. To handle this, we
			// store the key/value pairs and post process the metadata queries after
			// this `for` loop.
			if len(parts) > 1 && strings.Contains(parts[0], "metadata") {

				// The logic in this `if ... else if` utilizes order of search terms
				// to associate key/value search terms.
				//
				// Examples:
				//
				// 1. Search: ?q=metadata.key=foo
				//    Result: [{Key: "foo"}]
				//
				// 2. Search: ?q=metadata.value=bar
				//    Result: [{Value: "bar"}]
				//
				// 3. Search: ?q=metadata.key=foo%26metadata.value=bar
				//    Result: [{Key: "foo", Value: "bar"}]
				//
				// 4. Search: ?q=metadata.key=foo%26metadata.key=baz
				//    Result: [{Key: "foo"}, {Key: "baz"}]
				//
				// 5. Search: ?q=metadata.key=foo%26metadata.key=baz%26metadata.value=qux
				//    Result: [{Key: "foo"}, {Key: "baz", Value: "qux"}]
				if parts[0] == "metadata.key" {
					if len(metadata) > 0 && metadata[len(metadata)-1].Key == nil {
						metadata[len(metadata)-1].Key = &parts[1]
					} else {
						metadata = append(metadata, metaDatumQuery{Key: &parts[1]})
					}
				} else if parts[0] == "metadata.value" {
					if len(metadata) > 0 && metadata[len(metadata)-1].Value == nil {
						metadata[len(metadata)-1].Value = &parts[1]
					} else {
						metadata = append(metadata, metaDatumQuery{Value: &parts[1]})
					}
				}
			} else if len(parts) > 1 {

				// We are ignoring parts beyond the first two. A user could request
				// ?q=p1:p2:p3:p4. Instead of returning an error, we will just pick
				// out p1 and p2.

				// Create the match based on the first two parts.
				match = map[string]interface{}{
					// Use "query_string" because the "query" parameter supports
					// regular expressions.
					"query_string": map[string]interface{}{
						// The second part (`parts[1]`) contains the search string.
						"query": parts[1],
						// The first part (`parts[0]`) contains the search field.
						"fields": []string{strings.ToLower(parts[0])},
					},
				}
			} else {
				// Create the search based on a single part.
				match = map[string]interface{}{
					// Use "query_string" because the "query" parameter supports
					// regular expressions
					"query_string": map[string]interface{}{
						"query": parts[0],
					},
				}
			}

			// Add the match to the boolean query.
			if len(match) > 0 {
				must = append(must, match)
			}
		}

		// We have metadata in the query, which needs to be handled as a
		// nested query.
		if len(metadata) > 0 {

			// Add a boolean query for each metadata entry.
			for _, metadatum := range metadata {
				var fields []string
				var queryStr string

				if metadatum.Key != nil {
					fields = append(fields, "metadata.key")
					queryStr = *metadatum.Key
				}

				if metadatum.Value != nil {
					fields = append(fields, "metadata.value")
					// The "AND" keyword allows elasticsearch to query the "key" field
					// using the text before the "AND" clause and the "value" field
					// using the text after the "AND".
					if len(queryStr) > 0 {
						queryStr = queryStr + " AND "
					}
					queryStr += *metadatum.Value
				}

				// Create the match based on the first two parts.
				var match = map[string]interface{}{
					"nested": map[string]interface{}{
						"path": "metadata",
						"query": map[string]interface{}{
							// Use "query_string" because the "query" parameter supports
							// regular expressions.
							"query_string": map[string]interface{}{
								// The second part (`parts[1]`) contains the search string.
								"query": queryStr,
								// The first part (`parts[0]`) contains the search field.
								// "fields":[]string{strings.ToLower(parts[0])},
								"fields": fields,
							},
						},
					},
				}

				// Add the match to the boolean query.
				must = append(must, match)
			}
		}

		// Construct the whole query
		query = map[string]interface{}{
			"query": map[string]interface{}{
				"bool": map[string]interface{}{
					"must": must,
				},
			},
		}

	} else {
		// We will get here if the search is empty (`?q=`). In this case,
		// use `match_all`.
		query = map[string]interface{}{
			"query": map[string]interface{}{
				"match_all": map[string]interface{}{},
			},
		}
	}

	// Encode the search request.
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return nil, nil, gz.NewErrorMessageWithArgs(gz.ErrorUnexpected, err,
			[]string{"Error encoding search query"})
	}

	// Debug
	// fmt.Printf("* JSON encoded search[%s]\n", buf.String())

	// Send the search request to ElasticSearch, and get a response.
	res, err := globals.ElasticSearch.Search(
		globals.ElasticSearch.Search.WithContext(ctx),
		globals.ElasticSearch.Search.WithIndex(index),
		globals.ElasticSearch.Search.WithBody(&buf),
		globals.ElasticSearch.Search.WithTrackTotalHits(true),
		globals.ElasticSearch.Search.WithPretty(),
		globals.ElasticSearch.Search.WithFrom(
			int((gz.Max(pr.Page, 1)-1)*pr.PerPage)),
		globals.ElasticSearch.Search.WithSize(int(pr.PerPage)),
	)

	// Check to see if ElasticSearch returned an error.
	if err != nil {
		return nil, nil, gz.NewErrorMessageWithArgs(gz.ErrorUnexpected, err,
			[]string{"Error getting search response"})
	}

	defer res.Body.Close()

	// Check for error
	if res.IsError() {
		var errResult map[string]interface{}

		if err := json.NewDecoder(res.Body).Decode(&errResult); err != nil {
			return nil, nil, gz.NewErrorMessageWithArgs(gz.ErrorUnexpected, err,
				[]string{"Error parsing the search response error body"})
		}
		return nil, nil, gz.NewErrorMessageWithArgs(gz.ErrorUnexpected, err,
			[]string{"Search error ",
				errResult["error"].(map[string]string)["reason"]})
	}

	var elasticResult map[string]interface{}

	// Decode the search response
	if err := json.NewDecoder(res.Body).Decode(&elasticResult); err != nil {
		return nil, nil, gz.NewErrorMessageWithArgs(gz.ErrorUnexpected, err,
			[]string{"Error parsing the search response body"})
	}

	// Debug
	// Print the response status, number of results, and request duration.
	// fmt.Printf("* Search results [%s] %d hits; took: %dms\n",
	// res.Status(),
	// int(elasticResult["hits"].(map[string]interface{})["total"].(map[string]interface{})["value"].(float64)),
	// int(elasticResult["took"].(float64)))

	var result interface{}

	count := int64(0)
	if index == "fuel_models" {
		result, count = createModelResults(ctx, user, tx, elasticResult)
	} else if index == "fuel_worlds" {
		result, count = createWorldResults(ctx, user, tx, elasticResult)
	}

	// Get the total number of results.
	totalCount := int64(elasticResult["hits"].(map[string]interface{})["total"].(map[string]interface{})["value"].(float64))

	// Construct the pagination result
	page := gz.PaginationResult{}
	page.Page = pr.Page
	page.PerPage = pr.PerPage
	page.URL = pr.URL
	page.QueryCount = totalCount
	page.PageFound = count > 0 || (page.Page == 1 && count == 0)

	// Write the pagination headers
	gz.WritePaginationHeaders(page, w, r)

	// Debug
	// fmt.Printf("--- End of ElasticSearch ---\n")
	return result, &page, nil
}

func createWorldResults(ctx context.Context, user *users.User, tx *gorm.DB, elasticResult map[string]interface{}) (interface{}, int64) {
	// Construct the set of models
	worldsProto := fuel.Worlds{}
	var resourceIDs []int64

	// Build a list of resource ids
	for _, hit := range elasticResult["hits"].(map[string]interface{})["hits"].([]interface{}) {
		idString, ok := hit.(map[string]interface{})["_id"].(string)
		if ok && len(idString) > 0 {
			resourceID, err := strconv.ParseInt(idString, 10, 64)
			if err != nil {
				gz.LoggerFromContext(ctx).Error("Unable to convert ID to int64.", idString)
			}
			resourceIDs = append(resourceIDs, resourceID)
		} else {
			gz.LoggerFromContext(ctx).Error("Unable to convert ID to string.")
		}
	}

	// Get all the worlds from the DB and add them to the result
	var foundWorlds []worlds.World
	count := int64(0)
	// \todo: Add categories to world, and add back in `.Preload("Categories")` to the following line.
	if err := tx.Preload("Tags").Preload("License").Where(resourceIDs).Find(&foundWorlds).Error; err == nil {
		for _, world := range foundWorlds {

			if ok, _ := users.CheckPermissions(tx, *world.UUID, user, *world.Private, permissions.Read); ok {
				count++
				// Encode world into a protobuf message and add it to the list.
				fuelWorld := (&worlds.Service{}).WorldToProto(&world)
				worldsProto.Worlds = append(worldsProto.Worlds, fuelWorld)

				// Debug:
				// fmt.Printf("* Fuel world ID=%s, %s\n",
				// resourceID, hit.(map[string]interface{})["_source"])
			}
		}
	}

	return worldsProto, count
}

func createModelResults(ctx context.Context, user *users.User, tx *gorm.DB, elasticResult map[string]interface{}) (interface{}, int64) {
	// Construct the set of models
	var modelsProto fuel.Models
	var resourceIDs []int64

	// Build a list of resource ids
	for _, hit := range elasticResult["hits"].(map[string]interface{})["hits"].([]interface{}) {
		idString, ok := hit.(map[string]interface{})["_id"].(string)
		if ok && len(idString) > 0 {
			resourceID, err := strconv.ParseInt(idString, 10, 64)
			if err != nil {
				gz.LoggerFromContext(ctx).Error("Unable to convert ID to int64.", idString)
			}
			resourceIDs = append(resourceIDs, resourceID)
		} else {
			gz.LoggerFromContext(ctx).Error("Unable to convert ID to string.")

		}
	}

	// Get all the models from the DB and add them to the result
	var foundModels []*models.Model
	count := int64(0)
	if err := tx.Where(resourceIDs).Preload("Tags").Preload("Categories").Preload("License").Find(&foundModels).Error; err == nil {
		for _, model := range foundModels {
			if ok, _ := users.CheckPermissions(tx, *model.UUID, user, *model.Private, permissions.Read); ok {
				count++
				// Encode model into a protobuf message and add it to the list.
				fuelModel := (&models.Service{}).ModelToProto(model)
				modelsProto.Models = append(modelsProto.Models, fuelModel)
				// Debug:
				// fmt.Printf("* Fuel model ID=%s, %s\n",
				// resourceID, hit.(map[string]interface{})["_source"])
			}
		}
	}

	return &modelsProto, count
}
