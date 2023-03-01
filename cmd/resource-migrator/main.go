// The resource migrator allow us to migrate all the resources on saved on disk to a storage provider such as S3.
package main

import (
	"context"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	res "github.com/gazebo-web/fuel-server/bundles/common_resources"
	"github.com/gazebo-web/fuel-server/bundles/models"
	"github.com/gazebo-web/fuel-server/bundles/worlds"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/vcs"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/gazebo-web/gz-go/v7/storage"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"log"
	"os"
	"strconv"
)

func main() {
	// Initialize database
	cfg, err := gz.NewDatabaseConfigFromEnvVars()
	if err != nil {
		log.Fatalln("Failed to get MySQL database config:", err)
	}
	db, err := gz.InitDbWithCfg(&cfg)
	if err != nil {
		log.Fatalln("Failed to connect to MySQL database:", err)
	}

	// Set up git
	globals.VCSRepoFactory = func(ctx context.Context, dirpath string) vcs.VCS {
		return vcs.GoGitVCS{}.NewRepo(dirpath)
	}

	// Initialize S3 config
	s3session := session.Must(session.NewSession())
	s := storage.NewS3v1(s3.New(s3session), s3manager.NewUploader(s3session), "gz-fuel-staging-resources")

	// Upload all models available in the current instance
	err = uploadModels(s, db)
	if err != nil {
		log.Fatalln("Failed to migrate models:", err)
	}

	// Upload all worlds
	err = uploadWorlds(s, db)
	if err != nil {
		log.Fatalln("Failed to migrate models:", err)
	}

	log.Println("Successfully migrated all models and worlds")
}

// uploadWorlds uploads all the worlds found in the database.
func uploadWorlds(storage storage.Storage, db *gorm.DB) error {
	var list []worlds.World
	if err := db.Model(&worlds.World{}).Find(&list).Error; err != nil {
		return err
	}
	for _, world := range list {
		w := world
		if err := uploadResources(context.Background(), storage, "worlds", &w); err != nil {
			continue
		}
	}
	return nil
}

func uploadModels(storage storage.Storage, db *gorm.DB) error {
	var list models.Models
	if err := db.Model(&models.Model{}).Find(&list).Error; err != nil {
		return err
	}
	for _, model := range list {
		m := model
		if err := uploadResources(context.Background(), storage, "models", &m); err != nil {
			continue
		}
	}
	return nil
}

func uploadResources(ctx context.Context, storage storage.Storage, kind string, r res.Resource) error {
	v, err := uploadResource(ctx, storage, kind, "tip", r)
	if err != nil {
		return err
	}
	// If the tip version is not the version 1, we should migrate all the older versions
	for v > 1 {
		// Decrease by 1
		v--

		// Upload the resources for the current version
		v, err = uploadResource(ctx, storage, kind, strconv.Itoa(v), r)
		if err != nil {
			return err
		}
	}
	return nil
}

func uploadResource(ctx context.Context, storage storage.Storage, kind, version string, r res.Resource) (int, error) {
	path, ver, em := res.GetZip(ctx, r, kind, version)
	if em != nil {
		log.Printf("Failed to get zip file for %s: %s\n", kind, em.BaseError)

		return 0, em.BaseError
	}
	f, err := os.Open(*path)
	defer gz.Close(f)
	if err != nil {
		log.Printf("Failed to open zip file for %s: %s\n", kind, err)
		log.Printf("Name: %s | Owner: %s | Version: %d | Path: %d\n", *r.GetName(), *r.GetOwner(), ver, path)
		return 0, err
	}
	err = storage.UploadZip(ctx, res.CastResourceToStorageResource(r, uint64(ver)), f)
	if err != nil {
		log.Printf("Failed to upload zip file for %s: %s\n", kind, err)
		log.Printf("Name: %s | Owner: %s | Version: %d | Path: %d\n", *r.GetName(), *r.GetOwner(), ver, path)
		return 0, err
	}
	return ver, nil
}
