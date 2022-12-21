package worlds

// Import this file's dependencies
import (
	"context"
	"encoding/json"
	"github.com/elastic/go-elasticsearch/v8/esapi"
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
type worldElastic struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Metadata    []meta `json:"metadata,omitempty"`
	Owner       string `json:"owner"`
	Tags        string `json:"tags,omitempty"`
	Creator     string `json:"creator"`
}

// ElasticSearchRemoveWorld removes a world from elastic search
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
		gz.LoggerFromContext(ctx).Critical("Error getting response:", err)
	}
}

// ElasticSearchUpdateWorld will update ElasticSearch with a single world.
func ElasticSearchUpdateWorld(ctx context.Context, world World) {
	if globals.ElasticSearch == nil {
		return
	}

	// Construct the metadata information
	var metadata []meta
	for _, metadatum := range world.Metadata {
		metadata = append(metadata, meta{
			Key:   *metadatum.Key,
			Value: *metadatum.Value,
		})
	}

	// Construct the tag information
	var tagsBuilder strings.Builder
	for _, tag := range world.Tags {
		tagsBuilder.WriteString(*tag.Name)
		tagsBuilder.WriteString(` `)
	}

	// Build the ElasticSearch struct.
	m := worldElastic{
		Name:        *world.Name,
		Owner:       *world.Owner,
		Creator:     *world.Creator,
		Description: *world.Description,
		Tags:        tagsBuilder.String(),
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
		gz.LoggerFromContext(ctx).Critical("Error getting response:", err)
	}
	defer add.Body.Close()

	if add.IsError() {
		gz.LoggerFromContext(ctx).Error("[", add.Status(), "] Error indexing document ID:", world.ID)
	} else {
		// Deserialize the response into a map.
		var r map[string]interface{}
		if err := json.NewDecoder(add.Body).Decode(&r); err != nil {
			gz.LoggerFromContext(ctx).Error("Error parsing the response body:", err)
		} else {
			// Print the response status and indexed document version.
			gz.LoggerFromContext(ctx).Debug("[", add.Status(), "] ", r["result"], "; version:", int(r["_version"].(float64)))
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
		tx.Preload("Tags").Preload("Metadata").Find(&worlds)

		// Add each world to ElasticSearch.
		for _, world := range worlds {
			ElasticSearchUpdateWorld(ctx, world)
		}
	}
}
