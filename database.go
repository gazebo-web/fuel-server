package main

// Import this file's dependencies
import (
	"context"
	"encoding/xml"
	"fmt"
	"github.com/gosimple/slug"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/category"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/collections"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/license"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/models"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/subt"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/worlds"
	"gitlab.com/ignitionrobotics/web/fuelserver/globals"
	"gitlab.com/ignitionrobotics/web/ign-go"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jinzhu/gorm"
)

// DBAlterTables gives the option to alter database tables before migrating data
func DBAlterTables(ctx context.Context, db *gorm.DB) {

	// ALTER SubT Competition_Participants table, to change the Primary Key from 'owner' to 'id'. (if needed)
	found, err := indexIsPresent(db, "competition_participants", "idx_active_owner")
	if err != nil {
		ign.LoggerFromContext(ctx).Critical("Error with DB while checking index", err)
		log.Fatal("Error with DB while checking index", err)
		return
	}
	if !found {
		// We need to alter the table
		tx := db.Begin()
		tx.Exec("ALTER TABLE competition_participants DROP PRIMARY KEY;")
		tx.Exec("ALTER TABLE competition_participants ADD id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY FIRST;")
		tx.Commit()
	}
}

// DBMigrate auto migrates database tables
func DBMigrate(ctx context.Context, db *gorm.DB) {
	// Note about Migration from GORM doc: http://jinzhu.me/gorm/database.html#migration
	//
	// WARNING: AutoMigrate will ONLY create tables, missing columns and missing indexes,
	// and WON'T change existing column's type or delete unused columns to protect your data.
	//

	if db != nil {
		db.AutoMigrate(
			&license.License{},
			&category.Category{},
			&models.ModelMetadatum{},
			&models.Tag{},
			&ign.AccessToken{},
			&users.UniqueOwner{},
			&users.User{},
			&users.Organization{},
			&users.Team{},
			&collections.Collection{},
			&collections.CollectionAsset{},
			&models.Model{},
			&models.ModelDownload{},
			&models.ModelLike{},
			&models.ModelReport{},
			&worlds.World{},
			&worlds.WorldLike{},
			&worlds.WorldReport{},
			&worlds.WorldDownload{},
			&worlds.ModelInclude{},
			globals.Permissions.DBTable(),

			// SubT tables
			&subt.Portal{},
			&subt.LogFile{},
			&subt.Registration{},
			&subt.CompetitionParticipant{},
			&subt.CompetitionScore{},
		)
	}
}

// DBDropModels drops all tables from DB. Used by tests.
func DBDropModels(ctx context.Context, db *gorm.DB) {
	if db != nil {
		// First remove added FKs
		db.Model(&models.Model{}).RemoveForeignKey("owner", "unique_owners(name)")
		db.Model(&models.Model{}).RemoveForeignKey("creator", "users(username)")

		db.Model(&models.ModelReport{}).RemoveForeignKey("model", "models(model)")

		db.Model(&worlds.Worlds{}).RemoveForeignKey("owner", "unique_owners(name)")
		db.Model(&worlds.Worlds{}).RemoveForeignKey("creator", "users(username)")

		db.Model(&worlds.WorldReport{}).RemoveForeignKey("world", "worlds(world)")

		db.Model(&collections.Collection{}).RemoveForeignKey("owner", "unique_owners(name)")
		db.Model(&collections.Collection{}).RemoveForeignKey("creator", "users(username)")

		db.Model(&users.Organization{}).RemoveForeignKey("name", "unique_owners(name)")
		db.Model(&users.Organization{}).RemoveForeignKey("creator", "users(username)")

		db.Model(&users.Team{}).RemoveForeignKey("creator", "users(username)")
		db.Model(&users.User{}).RemoveForeignKey("username", "unique_owners(name)")
		// IMPORTANT NOTE: DROP TABLE order is important, due to FKs
		db.DropTableIfExists(

			// SubT tables
			&subt.Portal{},
			&subt.LogFile{},
			&subt.Registration{},
			&subt.CompetitionScore{},
			&subt.CompetitionParticipant{},

			// Fuel tables
			&license.License{},
			&models.ModelMetadatum{},
			&models.ModelReport{},
			&models.Model{},
			&models.ModelDownload{},
			&models.ModelLike{},
			&worlds.ModelInclude{},
			&worlds.WorldReport{},
			&worlds.World{},
			&worlds.WorldLike{},
			&worlds.WorldDownload{},
			&collections.CollectionAsset{},
			&collections.Collection{},
			&users.Team{},
			&users.Organization{},
			&users.User{},
			&users.UniqueOwner{},
			&models.Tag{},
			&category.Category{},
			globals.Permissions.DBTable(),
		)
		// Now also remove many_to_many tables, because they are not automatically removed.
		db.DropTableIfExists("model_tags", "world_tags", "model_categories")
	}
}

// LicDesc is used by DBAddDefaultData.
type LicDesc struct {
	name  string
	url   string
	image string
}

type CategoryDesc struct {
	name     string
	children []CategoryDesc
}

// DBAddDefaultData adds default data. Eg. Licenses.
func DBAddDefaultData(ctx context.Context, db *gorm.DB) {

	if db != nil {
		// Add default licenses
		defaultLicenses := []LicDesc{
			{"Creative Commons - Public Domain", "https://creativecommons.org/publicdomain/zero/1.0/",
				"https://i.creativecommons.org/p/88x31.png"},
			{"Creative Commons - Attribution", "http://creativecommons.org/licenses/by/4.0/",
				"https://i.creativecommons.org/l/by/4.0/88x31.png"},
			{"Creative Commons - Attribution - Share Alike", "http://creativecommons.org/licenses/by-sa/4.0/",
				"https://i.creativecommons.org/l/by-sa/4.0/88x31.png"},
			{"Creative Commons - Attribution - No Derivatives", "http://creativecommons.org/licenses/by-nd/4.0/",
				"https://i.creativecommons.org/l/by-nd/4.0/88x31.png"},
			{"Creative Commons - Attribution - Non Commercial", "http://creativecommons.org/licenses/by-nc/4.0/",
				"https://i.creativecommons.org/l/by-nc/4.0/88x31.png"},
			{"Creative Commons - Attribution - Non Commercial - Share Alike", "http://creativecommons.org/licenses/by-nc-sa/4.0/",
				"https://i.creativecommons.org/l/by-nc-sa/4.0/88x31.png"},
			{"Creative Commons - Attribution - Non Commercial - No Derivatives", "http://creativecommons.org/licenses/by-nc-nd/4.0/",
				"https://i.creativecommons.org/l/by-nc-nd/4.0/88x31.png"},
		}

		for _, l := range defaultLicenses {
			license := license.License{Name: &l.name, ContentURL: &l.url, ImageURL: &l.image}
			// This Create will return error if the value already exist.
			db.Create(&license)
		}
		defaultCategories := []CategoryDesc{
			{"Animals", []CategoryDesc{}},
			{"Architecture", []CategoryDesc{}},
			{"Cars and Vehicles", []CategoryDesc{
				{"Car Seat", []CategoryDesc{}},
			}},
			{"Electronics", []CategoryDesc{}},
			{"Fashion", []CategoryDesc{
				{"Bag", []CategoryDesc{}},
				{"Hat", []CategoryDesc{}},
				{"Shoe", []CategoryDesc{}},
				{"Sunglasses", []CategoryDesc{}},
				{"Watch", []CategoryDesc{}},
			}},
			{"Food and Drink", []CategoryDesc{
				{"Bottles, Cans, and Cups", []CategoryDesc{}},
				{"Perishables", []CategoryDesc{}},
			}},
			{"Furniture and Home", []CategoryDesc{
				{"Kitchen", []CategoryDesc{
					{"Appliance", []CategoryDesc{}},
				}},
			}},
			{"Music", []CategoryDesc{
				{"Guitar", []CategoryDesc{}},
			}},
			{"Nature and Plants", []CategoryDesc{}},
			{"People", []CategoryDesc{}},
			{"Places and Landscapes", []CategoryDesc{}},
			{"Robots", []CategoryDesc{}},
			{"Science and Technology", []CategoryDesc{
				{"Computer", []CategoryDesc{
					{"Keyboard", []CategoryDesc{}},
					{"Mouse", []CategoryDesc{}},
				}},
				{"Tablet and Smartphone", []CategoryDesc{}},
				{"Camera", []CategoryDesc{}},
				{"Headmounted", []CategoryDesc{
					{"Headphones", []CategoryDesc{}},
				}},
			}},
			{"Sports and Fitness", []CategoryDesc{}},
			{"Toys", []CategoryDesc{
				{"Action Figures", []CategoryDesc{}},
				{"Board Games", []CategoryDesc{}},
				{"Legos", []CategoryDesc{}},
				{"Stuffed Toys", []CategoryDesc{}},
			}},
		}
		createCategories(db, defaultCategories, nil)
	}
}

func createCategories(db *gorm.DB, categories []CategoryDesc, parentId *uint) {
	for _, c := range categories {
		newSlug := slug.Make(c.name)
		cat := category.Category{Name: &c.name, Slug: &newSlug, ParentID: parentId}
		db.Create(&cat)
		var record category.Category
		db.Where("name = ?", c.name).First(&record)
		createCategories(db, c.children, &record.ID)
	}
}

// DBAddCustomIndexes allows application to add custom indexes that cannot be added automatically
// by GORM.
func DBAddCustomIndexes(ctx context.Context, db *gorm.DB) {
	// TIP: command to check for existing foreign keys in db:
	// SELECT TABLE_NAME, COLUMN_NAME, CONSTRAINT_NAME, REFERENCED_TABLE_NAME, REFERENCED_COLUMN_NAME FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE WHERE REFERENCED_TABLE_SCHEMA = 'fuel';
	db.Model(&users.User{}).AddForeignKey("username", "unique_owners(name)", "RESTRICT", "RESTRICT")
	db.Model(&users.Organization{}).AddForeignKey("name", "unique_owners(name)", "RESTRICT", "RESTRICT")
	db.Model(&users.Organization{}).AddForeignKey("creator", "users(username)", "RESTRICT", "RESTRICT")
	db.Model(&users.Team{}).AddForeignKey("creator", "users(username)", "RESTRICT", "RESTRICT")

	db.Model(&models.Model{}).AddForeignKey("owner", "unique_owners(name)", "RESTRICT", "RESTRICT")
	db.Model(&models.Model{}).AddForeignKey("creator", "users(username)", "RESTRICT", "RESTRICT")

	db.Model(&models.ModelReport{}).AddForeignKey("model_id", "models(id)", "RESTRICT", "RESTRICT")

	db.Model(&worlds.Worlds{}).AddForeignKey("owner", "unique_owners(name)", "RESTRICT", "RESTRICT")
	db.Model(&worlds.Worlds{}).AddForeignKey("creator", "users(username)", "RESTRICT", "RESTRICT")

	db.Model(&worlds.WorldReport{}).AddForeignKey("world_id", "worlds(id)", "RESTRICT", "RESTRICT")

	db.Model(&collections.Collection{}).AddForeignKey("owner", "unique_owners(name)", "RESTRICT", "RESTRICT")
	db.Model(&collections.Collection{}).AddForeignKey("creator", "users(username)", "RESTRICT", "RESTRICT")

	// First add indexes for Models
	found, err := indexIsPresent(db, "models", "models_fultext")
	if err != nil {
		ign.LoggerFromContext(ctx).Critical("Error with DB while checking index", err)
		log.Fatal("Error with DB while checking index", err)
		return
	}
	if !found {
		db.Exec("ALTER TABLE models ADD FULLTEXT models_fultext (name, description);")
		db.Exec("ALTER TABLE tags ADD FULLTEXT tags_fultext (name);")
	}
	// TIP: You can check created indexes by executing in mysql: `show index from models;`

	// Now add indexes for Worlds
	found, err = indexIsPresent(db, "worlds", "worlds_fulltext")
	if err != nil {
		ign.LoggerFromContext(ctx).Critical("Error with DB while checking index", err)
		log.Fatal("Error with DB while checking index", err)
		return
	}
	if !found {
		db.Exec("ALTER TABLE worlds ADD FULLTEXT worlds_fulltext (name, description);")
		db.Exec("ALTER TABLE tags ADD FULLTEXT tags_fulltext (name);")
	}
	// Now add indexes for Collections
	found, err = indexIsPresent(db, "collections", "collections_fulltext")
	if err != nil {
		ign.LoggerFromContext(ctx).Critical("Error with DB while checking index", err)
		log.Fatal("Error with DB while checking index", err)
		return
	}
	if !found {
		db.Exec("ALTER TABLE collections ADD FULLTEXT collections_fulltext (name, description);")
	}
}

// indexIsPresent returns true if the index with name idxName already exists in the given table
func indexIsPresent(db *gorm.DB, table string, idxName string) (bool, error) {
	// Raw SQL
	rows, err := db.Raw("select * from information_schema.statistics where table_schema=database() and table_name=? and index_name=?;",
		table, idxName).Rows() //(*sql.Rows, error)
	defer rows.Close()
	if err != nil {
		return false, err
	}
	return rows.Next(), nil
}

// modelConfig is used by DBPopulate.
type modelConfig struct {
	Name        string   `xml:"name"`
	Description string   `xml:"description"`
	Version     string   `xml:"version"`
	SDF         []string `xml:"sdf"`
	AuthorList  []author `xml:"author"`
}

// author is used by DBPopulate.
type author struct {
	Name  string
	Email string
}

// DBPopulate populates the database with models from IGN_POPULATE_PATH.
func DBPopulate(ctx context.Context, path string, db *gorm.DB, onlyWhenEmpty bool) {
	// Users
	if hasTable := db.HasTable(&users.User{}); hasTable {
		var anonymousUser users.User
		// Get the user with the requested username
		db := globals.Server.Db
		db.Where("username = ?", "anonymous").First(&anonymousUser)
		if anonymousUser.Username == nil {
			// Create anonymous user, so no one else can create it
			username := "anonymous"
			name := "The anonymous user"
			email := "anonymous@osrf.org"
			identity := "test-identity"
			org := "OSRF"
			u := users.User{
				Name:         &name,
				Username:     &username,
				Email:        &email,
				Identity:     &identity,
				Organization: &org}
			if _, err := users.CreateUser(ctx, db, &u, false); err != nil {
				ign.LoggerFromContext(ctx).Error("Error creating anonymous user", err)
				log.Fatal("Error creating anonymous user", err)
			}
		}
	}

	// Models
	if hasTable := db.HasTable(&models.Model{}); hasTable {
		var models models.Models
		db.Find(&models)
		if len(models) > 0 && onlyWhenEmpty {
			return
		}
	}

	owner := "anonymous"
	var ownerDbUser users.User
	db.Where("username = ?", owner).First(&ownerDbUser)
	filepath.Walk(path,
		func(path string, f os.FileInfo, err error) error {
			if strings.Contains(path, "model.config") &&
				!strings.Contains(path, ".hg") {

				xmlFile, err := os.Open(path)
				if err != nil {
					return err
				}

				defer xmlFile.Close()
				b, _ := ioutil.ReadAll(xmlFile)

				var mc modelConfig
				xml.Unmarshal(b, &mc)

				fmt.Printf("Inserting Model %s\n", mc.Name)

				// Public permission
				permission := 0
				location := filepath.Dir(path)
				trimmedDesc := strings.TrimSpace(mc.Description)

				cm := models.CreateModel{Name: mc.Name, License: 1, Permission: permission,
					Description: trimmedDesc, Tags: "",
				}
				// Get a new UUID and model folder
				uuidStr, _, err := users.NewUUID(owner, "models")
				if err != nil {
					return err
				}
				(&models.Service{}).CreateModel(ctx, db, cm, uuidStr, location, &ownerDbUser)
			}
			return nil
		})
}
