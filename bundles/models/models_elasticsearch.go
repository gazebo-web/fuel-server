package models

// Import this file's dependencies
import (
	"context"
	"encoding/json"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/gazebo-web/fuel-server/bundles/category"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"
	"strconv"
	"strings"
)

// meta Contains a key-value pair
type meta struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// This is the structure of the  data will be stored in the fuel index.
type modelElastic struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Metadata    []meta `json:"metadata,omitempty"`
	Owner       string `json:"owner"`
	Tags        string `json:"tags,omitempty"`
	Categories  string `json:"categories"`
	Creator     string `json:"creator"`
	Collections string `json:"collections"`
}

// ElasticSearchRemoveModel removes a model from elastic search
func ElasticSearchRemoveModel(ctx context.Context, model *Model) {
	if globals.ElasticSearch == nil {
		return
	}

	// Set up the request object.
	req := esapi.DeleteRequest{
		Index:      "fuel_models",
		DocumentID: strconv.FormatUint(uint64(model.ID), 10),
		Refresh:    "true",
	}

	// Perform the request with the client.
	_, err := req.Do(context.Background(), globals.ElasticSearch)
	if err != nil {
		gz.LoggerFromContext(ctx).Critical("Error getting response:", err)
	}
}

// ElasticSearchUpdateModel will update ElasticSearch with a single model.
func ElasticSearchUpdateModel(ctx context.Context, tx *gorm.DB, model Model) {
	if globals.ElasticSearch == nil {
		return
	}

	// Construct the metadata information
	var metadata []meta
	for _, metadatum := range model.Metadata {
		metadata = append(metadata, meta{
			Key:   *metadatum.Key,
			Value: *metadatum.Value,
		})
	}

	// Get the name of each collection that this model belongs to.
	// We use a Raw SQL query because we can't use the collections_service due to
	// circular dependencies.
	type CollectionName struct {
		Name string
	}
	var collections []CollectionName
	tx.Raw("SELECT collections.name FROM collections INNER JOIN collection_assets ON collections.id = collection_assets.col_id WHERE collection_assets.asset_id=?;", model.ID).Scan(&collections)

	// Construct the collection information
	var collectionBuilder strings.Builder
	for i, col := range collections {
		collectionBuilder.WriteString(col.Name)
		if i+1 < len(collections) {
			collectionBuilder.WriteString(`, `)
		}
	}

	// Construct the tag information
	tags := strings.Join(TagsToStrSlice(model.Tags), " ")

	// Construct the category information
	categories := strings.Join(category.CategoriesToStrSlice(model.Categories), " ")

	// Build the ElasticSearch struct.
	m := modelElastic{
		Name:        *model.Name,
		Owner:       *model.Owner,
		Creator:     *model.Creator,
		Description: *model.Description,
		Tags:        tags,
		Categories:  categories,
		Collections: collectionBuilder.String(),
	}

	// Add in metadata
	if len(metadata) > 0 {
		m.Metadata = metadata
	}

	// Create the json representation
	jsonModel, _ := json.Marshal(&m)

	// Set up the request object.
	req := esapi.IndexRequest{
		Index:      "fuel_models",
		DocumentID: strconv.FormatUint(uint64(model.ID), 10),
		Body:       strings.NewReader(string(jsonModel)),
		Refresh:    "true",
	}

	// Perform the request with the client.
	add, err := req.Do(context.Background(), globals.ElasticSearch)
	if err != nil {
		gz.LoggerFromContext(ctx).Critical("Error getting response:", err)
	}
	defer add.Body.Close()

	if add.IsError() {
		gz.LoggerFromContext(ctx).Error("[", add.Status(), "] Error indexing document ID:", model.ID)
	} else {
		// Deserialize the response into a map.
		var r map[string]interface{}
		if err := json.NewDecoder(add.Body).Decode(&r); err != nil {
			gz.LoggerFromContext(ctx).Error("Error parsing the response body:", err)
		} else {
			// Print the response status and indexed document version.
			gz.LoggerFromContext(ctx).Debug("[", add.Status(), "] ", r["result"], "; version=", int(r["_version"].(float64)))
		}
	}
}

// ElasticSearchUpdateAll will update ElasticSearch with all the models in the
// SQL database.
func ElasticSearchUpdateAll(ctx context.Context, tx *gorm.DB) {
	if globals.ElasticSearch == nil {
		return
	}

	// Make sure that we have a Model table.
	if hasTable := tx.HasTable(&Model{}); hasTable {
		var models Models

		// Get all the models
		tx.Preload("Tags").Preload("Metadata").Preload("Categories").Find(&models)

		// TODO: Use the Bulk ElasticSearch API.

		// Add each model to ElasticSearch.
		for _, model := range models {
			ElasticSearchUpdateModel(ctx, tx, model)
		}
	}
}
