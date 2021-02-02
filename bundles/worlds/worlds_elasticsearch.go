package worlds

// Import this file's dependencies
import (
	"context"
	"encoding/json"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/globals"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/category"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/common_resources"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"strconv"
	"strings"
)

// This is the structure of the  data will be stored in the fuel index.
type worldElastic struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Metadata    commonres.Metadata `json:"metadata,omitempty"`
	Owner       string `json:"owner"`
	Tags        string `json:"tags,omitempty"`
	Categories  string `json:"categories"`
	Creator     string `json:"creator"`
	Collections string `json:"collections"`
}

//ElasticSearchRemoveWorld removes a world from elastic search
func ElasticSearchRemoveWorld(ctx context.Context, world *World) {
	if globals.ElasticSearch == nil {
		return
	}

	// Set up the request object.
	req := esapi.DeleteRequest{
		Index:      "fuel_worlds",
		DocumentID: strconv.FormatUint(uint64(world.ID), 10),
		Refresh:    "true",
	}

	// Perform the request with the client.
	_, err := req.Do(context.Background(), globals.ElasticSearch)
	if err != nil {
		ign.LoggerFromContext(ctx).Critical("Error getting response:", err)
	}
}

// ElasticSearchUpdateWorld will update ElasticSearch with a single world.
func ElasticSearchUpdateWorld(ctx context.Context, tx *gorm.DB, world World) {
	if globals.ElasticSearch == nil {
		return
	}

	// Construct the metadata information
	var metadata commonres.Metadata
	for _, metadatum := range world.Metadata {
		metadata = append(metadata, commonres.Metadatum{
			Key:   metadatum.Key,
			Value: metadatum.Value,
		})
	}

	// Get the name of each collection that this world belongs to.
	// We use a Raw SQL query because we can't use the collections_service due to
	// circular dependencies.
	type CollectionName struct {
		Name string
	}
	var collections []CollectionName
	tx.Raw("SELECT collections.name FROM collections INNER JOIN collection_assets ON collections.id = collection_assets.col_id WHERE collection_assets.asset_id=?;", world.ID).Scan(&collections)

	// Construct the collection information
	var collectionBuilder strings.Builder
	for i, col := range collections {
		collectionBuilder.WriteString(col.Name)
		if i+1 < len(collections) {
			collectionBuilder.WriteString(`, `)
		}
	}

	// Construct the tag information
	tags := strings.Join(commonres.TagsToStrSlice(world.Tags), " ")

	// Construct the category information
	categories := strings.Join(category.CategoriesToStrSlice(world.Categories), " ")

	// Build the ElasticSearch struct.
	m := worldElastic{
		Name:        *world.Name,
		Owner:       *world.Owner,
		Creator:     *world.Creator,
		Description: *world.Description,
		Tags:        tags,
		Categories:  categories,
		Collections: collectionBuilder.String(),
	}

	// Add in metadata
	if len(metadata) > 0 {
		m.Metadata = metadata
	}

	// Create the json representation
	jsonWorld, _ := json.Marshal(&m)

	// Set up the request object.
	req := esapi.IndexRequest{
		Index:      "fuel_worlds",
		DocumentID: strconv.FormatUint(uint64(world.ID), 10),
		Body:       strings.NewReader(string(jsonWorld)),
		Refresh:    "true",
	}

	// Perform the request with the client.
	add, err := req.Do(context.Background(), globals.ElasticSearch)
	if err != nil {
		ign.LoggerFromContext(ctx).Critical("Error getting response:", err)
	}
	defer add.Body.Close()

	if add.IsError() {
		ign.LoggerFromContext(ctx).Error("[", add.Status(), "] Error indexing document ID:", world.ID)
	} else {
		// Deserialize the response into a map.
		var r map[string]interface{}
		if err := json.NewDecoder(add.Body).Decode(&r); err != nil {
			ign.LoggerFromContext(ctx).Error("Error parsing the response body:", err)
		} else {
			// Print the response status and indexed document version.
			ign.LoggerFromContext(ctx).Debug("[", add.Status(), "] ", r["result"], "; version:", int(r["_version"].(float64)))
		}
	}
}

// ElasticSearchUpdateAll will update ElasticSearch with all the worlds in the
// SQL database.
func ElasticSearchUpdateAll(ctx context.Context, tx *gorm.DB) {
	if globals.ElasticSearch == nil {
		return
	}

	// Make sure that we have a World table.
	if hasTable := tx.HasTable(&World{}); hasTable {
		var worlds Worlds

		// Get all the worlds
		tx.Preload("Tags").Preload("Metadata").Preload("Categories").Find(&worlds)

		// Add each world to ElasticSearch.
		for _, world := range worlds {
			ElasticSearchUpdateWorld(ctx, tx, world)
		}
	}
}
