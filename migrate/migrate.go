package migrate

import (
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/collections"
	res "gitlab.com/ignitionrobotics/web/fuelserver/bundles/common_resources"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/models"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/subt"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/worlds"
	"gitlab.com/ignitionrobotics/web/fuelserver/globals"
	"gitlab.com/ignitionrobotics/web/fuelserver/permissions"
	"gitlab.com/ignitionrobotics/web/fuelserver/vcs"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"context"
	"github.com/jinzhu/gorm"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// CollectionsSetDefaultLocation iterates over existing collection DB records
// and sets the location to those collections having Location = nil.
func CollectionsSetDefaultLocation(ctx context.Context, db *gorm.DB) {
	tx := db.Begin()
	var colList collections.Collections
	if err := tx.Model(&collections.Collection{}).Where("location IS NULL").
		Find(&colList).Error; err != nil {
		tx.Rollback()
		log.Fatal("[MIGRATION] Error finding collections to set default location", err)
	}
	for _, col := range colList {
		if col.Location == nil {
			loc := users.GetResourcePath(*col.GetOwner(), *col.GetUUID(), "collections")
			os.MkdirAll(loc, 0711)
			tx.Model(&col).Update("Location", &loc)
			_, em := res.CreateResourceRepo(ctx, &col, *col.GetLocation())
			if em != nil {
				tx.Rollback()
				log.Fatalf("[MIGRATION] Error initializing repo at (%s).", *col.GetLocation())
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		log.Fatal("[MIGRATION] Error during 'CollectionsSetDefaultLocation' commit TX", err)
	}
	log.Println("[MIGRATION] Finished 'CollectionsSetDefaultLocation' migration script")
}

// RecomputeZipFileSizes updates all models and worlds and set them with the
// latest zip's file size.
func RecomputeZipFileSizes(ctx context.Context, db *gorm.DB) {
	migrate, _ := ign.ReadEnvVar("IGN_FUEL_MIGRATE_RESET_ZIP_FILESIZE")
	if value, err := strconv.ParseBool(migrate); err != nil || !value {
		if err != nil {
			log.Printf("Error parsing IGN_FUEL_MIGRATE_RESET_ZIP_FILESIZE. Got value: %s. Error: %s", migrate, err)
		}
		return
	}
	log.Println("[MIGRATION] Running 'Recompute Zip file sizes' migration script")
	if _, err := os.Stat(globals.ResourceDir); err != nil {
		log.Fatal("[MIGRATION] globals.ResourceDir does not exist. Ignoring script",
			globals.ResourceDir)
	}

	tx := db.Begin()
	var modelList models.Models
	if err := tx.Model(&models.Model{}).Find(&modelList).Error; err != nil {
		tx.Rollback()
		log.Fatal("[MIGRATION] Error finding models to recompute file sizes", err)
	}
	for _, model := range modelList {
		if _, err := os.Stat(*model.GetLocation()); err != nil {
			log.Printf("[MIGRATION] Model folder (%s) does not exist. Ignoring zip file",
				*model.GetLocation())
			continue
		}
		modelsFolder := filepath.Join(globals.ResourceDir, *model.GetOwner(), "models")
		os.MkdirAll(modelsFolder, 0711)

		zipPath, _, em := res.GetZip(ctx, &model, "models", "tip")
		if em != nil {
			tx.Rollback()
			log.Fatalf("[MIGRATION] Error during recompute file sizes. Base error: %s. Model: %v",
				em.BaseError, model)
		}
		fInfo, err := os.Stat(*zipPath)
		if err != nil {
			tx.Rollback()
			log.Fatal("[MIGRATION] Error during recompute file sizes", err, *zipPath)
		}
		newSize := int(fInfo.Size())
		tx.Model(&model).Update("Filesize", newSize)
	}

	var worldList worlds.Worlds
	if err := tx.Model(&worlds.World{}).Unscoped().Find(&worldList).Error; err != nil {
		log.Fatal("[MIGRATION] Error finding worlds to recompute file sizes", err)
	}
	for _, w := range worldList {
		if _, err := os.Stat(*w.GetLocation()); err != nil {
			log.Printf("[MIGRATION] World folder (%s) does not exist. Ignoring zip file",
				*w.GetLocation())
			continue
		}
		worldsFolder := filepath.Join(globals.ResourceDir, *w.GetOwner(), "worlds")
		os.MkdirAll(worldsFolder, 0711)

		zipPath, _, em := res.GetZip(ctx, &w, "worlds", "tip")
		if em != nil {
			tx.Rollback()
			log.Fatal("[MIGRATION] Error during recompute file sizes", em.BaseError, w)
		}
		fInfo, err := os.Stat(*zipPath)
		if err != nil {
			tx.Rollback()
			log.Fatal("[MIGRATION] Error during recompute file sizes", err, *zipPath)
		}
		newSize := int(fInfo.Size())
		tx.Model(&w).Update("Filesize", newSize)
	}

	if err := tx.Commit().Error; err != nil {
		log.Fatal("[MIGRATION] Error during 'recompute file sizes' commit TX", err)
	}

	log.Println("[MIGRATION] Successfully finished 'Recompute file sizes' migration script")
}

// RecomputeDownloadsAndLikes is a migrate script used to reset Models
// and Worlds' 'Downloads' and 'Likes' count fields, based on the result of
// counting how many records exist in model_downloads and model_likes tables
// (and their worlds counterparts).
// NOTE: This script is expected to be run just once on each server.
func RecomputeDownloadsAndLikes(ctx context.Context, db *gorm.DB) {
	migrate, _ := ign.ReadEnvVar("IGN_FUEL_MIGRATE_RESET_LIKE_AND_DOWNLOADS")
	if value, err := strconv.ParseBool(migrate); err != nil || !value {
		if err != nil {
			log.Printf("Error parsing IGN_FUEL_MIGRATE_RESET_LIKE_AND_DOWNLOADS. Got value: %s. Error: %s", migrate, err)
		}
		return
	}
	log.Println("[MIGRATION] Running 'Recompute Downloads And Likes' migration script")
	tx := db.Begin()

	if em := (&models.Service{}).ComputeAllCounters(tx); em != nil {
		tx.Rollback()
		log.Fatal("[MIGRATION] Error while recomputing likes and downloads", em.BaseError)
	}
	if em := (&worlds.Service{}).ComputeAllCounters(tx); em != nil {
		tx.Rollback()
		log.Fatal("[MIGRATION] Error while recomputing likes and downloads", em.BaseError)
	}

	if err := tx.Commit().Error; err != nil {
		log.Fatal("[MIGRATION] Error while recomputing likes and downloads", err)
	}
	log.Println("[MIGRATION] Successfully finished 'Recompute Downloads And Likes' migration script")
}

// MakeResourcesPublicWhenNotSet updates models and worlds that do not have their
// private field set (ie. is NULL) to be public instead (ie. private = 0).
func MakeResourcesPublicWhenNotSet(ctx context.Context, db *gorm.DB) {
	log.Println("[MIGRATION] making models and worlds Public when private is null")
	tx := db.Begin()

	if err := tx.Exec("UPDATE models SET private = 0 WHERE private IS NULL;").Error; err != nil {
		tx.Rollback()
		log.Fatal("[MIGRATION] Error while running 'UPDATE models SET private = 0 WHERE private IS NULL;'", err)
	}
	if err := tx.Exec("UPDATE worlds SET private = 0 WHERE private IS NULL;").Error; err != nil {
		tx.Rollback()
		log.Fatal("[MIGRATION] Error while running 'UPDATE worlds SET private = 0 WHERE private IS NULL;'", err)
	}

	if err := tx.Commit().Error; err != nil {
		log.Fatal("[MIGRATION] Error while making models and worlds Public", err)
	}
}

// CasbinPermissions adds read/write permissions to owners of existent Models
// and Worlds.
// NOTE: This script is expected to be run just once on each server.
func CasbinPermissions(ctx context.Context, db *gorm.DB) {
	migrate, _ := ign.ReadEnvVar("IGN_FUEL_MIGRATE_CASBIN")
	if value, err := strconv.ParseBool(migrate); err != nil || !value {
		if err != nil {
			log.Printf("Error parsing IGN_FUEL_MIGRATE_CASBIN. Got value: %s. Error: %s", migrate, err)
		}
		return
	}
	log.Println("[MIGRATION] Running Casbin Permissions migration script")
	q := db

	// Create Groups for existing organizations
	var orgs users.Organizations
	if err := q.Model(&users.Organization{}).Unscoped().Find(&orgs).Error; err != nil {
		log.Fatal("[MIGRATION] Error finding organizations to create groups", err)
	}
	for _, org := range orgs {
		globals.Permissions.AddUserGroupRole(*org.Creator, *org.Name, permissions.Owner)
	}

	var modelList models.Models
	if err := q.Model(&models.Model{}).Unscoped().Find(&modelList).Error; err != nil {
		log.Fatal("[MIGRATION] Error finding models to add permissions", err)
	}
	for _, model := range modelList {
		// add read and write permissions
		globals.Permissions.AddPermission(*model.Owner, *model.UUID, permissions.Read)
		globals.Permissions.AddPermission(*model.Owner, *model.UUID, permissions.Write)
	}

	var worldList worlds.Worlds
	if err := q.Model(&worlds.World{}).Unscoped().Find(&worldList).Error; err != nil {
		log.Fatal("[MIGRATION] Error finding worlds to add permissions", err)
	}
	for _, w := range worldList {
		// add read and write permissions
		globals.Permissions.AddPermission(*w.Owner, *w.UUID, permissions.Read)
		globals.Permissions.AddPermission(*w.Owner, *w.UUID, permissions.Write)
	}
}

// ToUniqueNamesWithForeignKeys - migrate users and orgazanizations names to
// unique_owners table. This allows to then create foreign keys on unique names.
// This migration script only runs if the IGN_FUEL_MIGRATE_UNIQUEOWNERS_TABLE
// env var is set with value 'true'.
// NOTE: This script is expected to be run just once on each server.
func ToUniqueNamesWithForeignKeys(ctx context.Context, db *gorm.DB) {
	migrate, _ := ign.ReadEnvVar("IGN_FUEL_MIGRATE_UNIQUEOWNERS_TABLE")
	if value, err := strconv.ParseBool(migrate); err != nil || !value {
		if err != nil {
			log.Printf("Error parsing IGN_FUEL_MIGRATE_UNIQUEOWNERS_TABLE. Got value: %s. Error: %s", migrate, err)
		}
		return
	}
	log.Println("[MIGRATION] Running DB unique_owners migration script")

	tx := db.Begin()

	if err := tx.Exec("UPDATE models SET creator = owner WHERE creator IS NULL;").Error; err != nil {
		tx.Rollback()
		log.Fatal("[MIGRATION] Error while running 'UPDATE models SET creator = owner WHERE creator IS NULL;'", err)
	}
	if err := tx.Exec("UPDATE worlds SET creator = owner WHERE creator IS NULL;").Error; err != nil {
		tx.Rollback()
		log.Fatal("[MIGRATION] Error while running 'UPDATE worlds SET creator = owner WHERE creator IS NULL;'", err)
	}

	var ownerDbUser users.User
	// First check for openrobotics user
	err := tx.Where("username = ?", "openrobotics").First(&ownerDbUser).Error
	if err != nil {
		// otherwise, check if anonymous user exist
		err = tx.Where("username = ?", "anonymous").First(&ownerDbUser).Error
	}
	if err != nil {
		// otherwise, check if anonymous user exist
		err = tx.Where("id = ?", "1").First(&ownerDbUser).Error
	}
	if err == nil {
		// only update organizations table if we found a default user
		if err := tx.Exec("UPDATE organizations SET creator = ? WHERE creator IS NULL;", *ownerDbUser.Username).Error; err != nil {
			tx.Rollback()
			log.Fatal("[MIGRATION] Error while running 'UPDATE organizations SET creator = ? WHERE creator IS NULL'", err)
		}
	}

	var counter int
	// Count the number of unique_owners, to see if if was already initialized.
	if err := tx.Model(&users.UniqueOwner{}).Count(&counter).Error; err != nil {
		log.Fatal("[MIGRATION] Error while running migration ToUniqueNamesWithForeignKeys", err)
	}
	if counter == 0 {
		if err := tx.Exec("INSERT INTO unique_owners (name, created_at, updated_at, deleted_at, owner_type) SELECT username, created_at, updated_at, deleted_at, 'users' FROM users;").Error; err != nil {
			tx.Rollback()
			log.Fatal("[MIGRATION] Error while running 'INSERT INTO unique_owners from USERS'", err)
		}
		if err := tx.Exec("INSERT INTO unique_owners (name, created_at, updated_at, deleted_at, owner_type) SELECT name, created_at, updated_at, deleted_at, 'organizations' FROM organizations;").Error; err != nil {
			tx.Rollback()
			log.Fatal("[MIGRATION] Error while running 'INSERT INTO unique_owners from ORGANIZATIONS'", err)
		}
	}
	if err := tx.Commit().Error; err != nil {
		log.Fatal("[MIGRATION] Error while trying to commit all changes", err)
	}

	// Command to check for existing foreign keys in db:
	// SELECT TABLE_NAME, COLUMN_NAME, CONSTRAINT_NAME, REFERENCED_TABLE_NAME, REFERENCED_COLUMN_NAME FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE WHERE REFERENCED_TABLE_SCHEMA = 'fuel';
}

// ToModelGitRepositories - migrate to git repositories does 2 things:
// 1) Migrate from HG to GIT when needed.
// 2) Tag a model repository with its UUID, if needed.
// ToModelGitRepositories only runs if the IGN_FUEL_MIGRATE_MODEL_REPOSITORIES
// env var is set with value 'true'.
// NOTE: This script is expected to be run just once on each server.
func ToModelGitRepositories(ctx context.Context) {
	// Do we need to run migration logic?
	migrate, _ := ign.ReadEnvVar("IGN_FUEL_MIGRATE_MODEL_REPOSITORIES")
	if value, err := strconv.ParseBool(migrate); err != nil || !value {
		if err != nil {
			log.Printf("Error parsing IGN_FUEL_MIGRATE_MODEL_REPOSITORIES. Got value: %s. Error: %s", migrate, err)
		}
		return
	}
	log.Println("[MIGRATION] Running MODEL MIGRATION SCRIPTS")
	root := globals.ResourceDir

	re := regexp.MustCompile(root + "/([^/]+)/models/([^/]+)")

	migrateFn := func(path string, f os.FileInfo, err error) error {
		// is is a model folder?
		match := re.FindStringSubmatch(path)
		if match == nil || match[0] != path || match[2] == ".zips" {
			log.Println("Not a Model folder match", re, path)
			return nil
		}
		log.Println("Match! It is a model folder", path, match[1], match[2])
		theUUID := match[2]
		needsTag := false

		repo := vcs.GoGitVCS{}.NewRepo(path)

		// backward compatibility: was this a folder created with mercurial?
		gitFolder := filepath.Join(path, ".git")
		if _, err := os.Stat(gitFolder); err != nil {
			// .git does not exist... Let's make it a git repo!
			// Convert it to "git".
			log.Println("Switching to GIT: " + path)
			if err := repo.InitRepo(ctx); err != nil {
				ign.LoggerFromContext(ctx).Error("Error migrating to GIT. Path " + path)
				panic("Error migrating to GIT. Path " + path)
			}
			needsTag = true
		}

		// now check if the git repo was tagged with model's UUID
		if !needsTag {
			hasTag, err := repo.(*vcs.GoGitVCS).HasTag(theUUID)
			if err != nil {
				ign.LoggerFromContext(ctx).Error("Error while checking for UUID Tag existence. Path " + path)
				panic("Error while checking for UUID Tag existence. Path " + path)
			}
			log.Println("Found UUID tag?", hasTag)
			needsTag = !hasTag
		}

		// Tag the git repo if needed
		if needsTag {
			// Also tag the repo with the UUID. The UUID is the last segment of the path
			log.Println("Tagging repo with its UUID: " + path)
			if err := repo.Tag(ctx, theUUID); err != nil {
				ign.LoggerFromContext(ctx).Error("Error while tagging GIT repo. Path " + path)
				panic("Error while tagging GIT repo. Path " + path)
			}
		}
		// All OK. Return skip dir so it does not continue walking this
		// folder's contents.
		return filepath.SkipDir
	}

	// Run the walk function
	if err := filepath.Walk(root, migrateFn); err != nil {
		ign.LoggerFromContext(ctx).Error("Error while migrating model repositories", err)
		panic("Error while migrating model repositories")
	}
}

// LogFileScoresToCompetitionScore creates CompetitionScore entries from existing log file scores.
func LogFileScoresToCompetitionScore(db *gorm.DB, circuit string) {
	log.Println("[MIGRATION] Running 'Log File Scores To Competition Scores' migration script.")
	tx := db.Begin()

	// Only migrate if no competition score entries exist
	var count int
	if err := tx.Model(&subt.CompetitionScore{}).Count(&count).Error; err != nil {
		log.Printf("[MIGRATION] Could not get existing competition score count.")
		tx.Rollback()
		return
	}
	if count != 0 {
		log.Println("[MIGRATION] Previously defined competition scores found. Skipping log file score migration.")
		return
	}

	// Migrate scores from log files
	var logs subt.LogFiles
	if err := tx.Model(&subt.LogFiles{}).Scan(&logs).Error; err != nil {
		log.Fatal("[MIGRATION] Could not get log file entries.", err)
		return
	}
	// Create a CompetitionScore entry for each log file with a score
	for _, log := range logs {
		if log.Score == nil {
			continue
		}
		score := float64(*log.Score)
		tx.Model(&subt.CompetitionScore{}).Create(&subt.CompetitionScore{
			GroupID:     log.UUID,
			Competition: log.Competition,
			Circuit:     &circuit,
			Owner:       log.Owner,
			Score:       &score,
			Sources:     log.UUID,
		})
	}

	if err := tx.Commit().Error; err != nil {
		log.Fatal("[MIGRATION] Error during 'LogFileScoresToCompetitionScore' commit TX.", err)
	}
}
