// The resource migrator allow us to migrate all the resources saved on disk to a storage provider such as S3.
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
	"github.com/schollz/progressbar/v3"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const (
	maxParallelUploads = 15
)

func main() {
	s, db := setup()
	defer gz.Close(db)

	run(s, db)
}

func run(s storage.Storage, db *gorm.DB) {
	started := time.Now()

	// Get the list of resources to upload for: Models
	ms, modelListSize, err := getResourceToUpload[*models.Model](db)
	if err != nil {
		log.Panicln("Failed to get models:", err)
	}

	// Get the list of resources to upload for: Worlds
	ws, worldListSize, err := getResourceToUpload[*worlds.World](db)
	if err != nil {
		log.Panicln("Failed to get worlds:", err)
	}
	totalSize := modelListSize + worldListSize

	// Channel to request resource uploads
	c := make(chan uploadRequest, totalSize)

	// Channel to handle errors
	e := make(chan errorUploading, totalSize)

	// Requesting all models
	log.Println("Processing Models")
	requestUpload[*models.Model](c, ms, "models")

	// Requesting all worlds
	log.Println("Processing Worlds")
	requestUpload[*worlds.World](c, ws, "worlds")

	// Listen for exit signal from when all resources have been uploaded
	exit := make(chan struct{}, 1)

	// Begin parallel uploads and keep track of them using the progress bar. It will send a single item to the exit
	// channel once it finished
	bar := newProgressBar(totalSize, exit)
	for i := 0; i < maxParallelUploads; i++ {
		go upload(c, e, s, bar)
	}

	// Listen for Interrupt and Terminate signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	fails := make([]errorUploading, 0, modelListSize+worldListSize)

	for {
		select {
		case failed := <-e:
			fails = append(fails, failed)
		case sig := <-sigs:
			log.Panicln("Signal received:", sig.String())
		case <-exit:
			log.Println("Models and Worlds were migrated. Took:", time.Since(started).Seconds(), "seconds")
			if len(fails) > 0 {
				log.Printf("However, the following resources returned an error while uploading them:")
				for _, fail := range fails {
					log.Printf("Resource [%s]: %s/%s - Error: %s\n", fail.Request.Kind,
						*fail.Request.Resource.GetOwner(), *fail.Request.Resource.GetName(), fail.Error)
				}
			}
			return
		}
	}
}

func setup() (storage.Storage, *gorm.DB) {
	db, err := setupDB()
	if err != nil {
		log.Fatalln("Failed to set up to MySQL database conn:", err)
	}

	globals.ResourceDir = os.Getenv("FUEL_RESOURCE_DIR")

	// Set up git
	globals.VCSRepoFactory = func(ctx context.Context, dirpath string) vcs.VCS {
		return vcs.GoGitVCS{}.NewRepo(dirpath)
	}

	// Initialize S3 config
	s3session := session.Must(session.NewSession())
	s := storage.NewS3v1(s3.New(s3session), s3manager.NewUploader(s3session), os.Getenv("AWS_S3_BUCKET"))
	return s, db
}

func newProgressBar(size int, exit chan struct{}) *progressbar.ProgressBar {
	return progressbar.NewOptions(size,
		progressbar.OptionSetPredictTime(false),
		progressbar.OptionShowCount(),
		progressbar.OptionSetDescription("Uploading resources"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetWidth(10),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionOnCompletion(func() {
			exit <- struct{}{}
		}),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
	)
}

func setupDB() (*gorm.DB, error) {
	// Initialize database
	cfg, err := gz.NewDatabaseConfigFromEnvVars()
	if err != nil {
		return nil, err
	}
	db, err := gz.InitDbWithCfg(&cfg)
	if err != nil {
		return nil, err
	}
	return db, err
}

type uploadRequest struct {
	Kind     string
	Resource res.Resource
}

type errorUploading struct {
	Error   error
	Request uploadRequest
}

func requestUpload[T res.Resource](c chan uploadRequest, items []T, kind string) {
	for _, item := range items {
		c <- uploadRequest{
			Kind:     kind,
			Resource: item,
		}
	}
}

func upload(c chan uploadRequest, e chan errorUploading, storage storage.Storage, bar *progressbar.ProgressBar) {
	for !bar.IsFinished() {
		req := <-c
		err := uploadResources(context.Background(), storage, req.Kind, req.Resource)
		if err != nil {
			e <- errorUploading{
				Error:   err,
				Request: req,
			}
		}
		_ = bar.Add(1)
	}
}

func getResourceToUpload[T res.Resource](db *gorm.DB) ([]T, int, error) {
	var list []T
	var model T
	if err := db.Model(&model).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, len(list), nil
}

func uploadResources(ctx context.Context, storage storage.Storage, kind string, r res.Resource) error {
	v, err := uploadResource(ctx, storage, kind, "tip", r)
	if err != nil {
		return err
	}
	// If the tip versions is not the versions 1, we should migrate all the older versions
	for v > 1 {
		// Decrease by 1
		v--

		// Upload the resources for the current versions
		v, err = uploadResource(ctx, storage, kind, strconv.Itoa(v), r)
		if err != nil {
			return err
		}
	}
	return nil
}

func uploadResource(ctx context.Context, storage storage.Storage, kind, versions string, r res.Resource) (int, error) {
	path, ver, em := res.GetZip(ctx, r, kind, versions)
	if em != nil {
		log.Printf("Failed to get zip file for %s: %s\n", kind, em.BaseError)

		return 0, em.BaseError
	}
	f, err := os.Open(*path)
	defer gz.Close(f)
	if err != nil {
		log.Printf("Failed to open zip file for %s: %s\n", kind, err)
		log.Printf("Name: %s | Owner: %s | versions: %d | Path: %d\n", *r.GetName(), *r.GetOwner(), ver, path)
		return 0, err
	}
	err = storage.UploadZip(ctx, res.CastResourceToStorageResource(r, uint64(ver)), f)
	if err != nil {
		log.Printf("Failed to upload zip file for %s: %s\n", kind, err)
		log.Printf("Name: %s | Owner: %s | versions: %d | Path: %d\n", *r.GetName(), *r.GetOwner(), ver, path)
		return 0, err
	}
	return ver, nil
}
