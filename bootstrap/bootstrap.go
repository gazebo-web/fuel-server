// Package bootstrap is the startup both fuel-server binaries share: the
// web server (package main) and the AI Builder agent (cmd/aibuilder). It
// sets the globals that the bundles and aitools rely on: the database and
// JWT key (globals.Server), the resource directory, the VCS factory,
// permissions, the query cache and file storage.
//
// Each step is a separate function so the web server can keep its own
// order (routes and migrations run between them); the agent calls Shared.
package bootstrap

import (
	"context"
	"log"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/bradfitz/gomemcache/memcache"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/gazebo-web/gz-go/v7/storage"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"

	"github.com/gazebo-web/fuel-server/bundles/subt"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/permissions"
	"github.com/gazebo-web/fuel-server/vcs"
)

// SysAdminForTest is the system administrator under `go test`.
const SysAdminForTest = "rootfortests"

// IsGoTest reports whether this process is a test binary.
func IsGoTest() bool {
	return strings.Contains(strings.ToLower(os.Args[0]), "test")
}

// Logger returns the startup logger (IGN_FUEL_VERBOSITY) and a context
// carrying it.
func Logger(name string) (gz.Logger, context.Context) {
	verbosity := gz.VerbosityWarning
	if verbStr, verr := gz.ReadEnvVar("IGN_FUEL_VERBOSITY"); verr == nil {
		verbosity, _ = strconv.Atoi(verbStr)
	}
	logger := gz.NewLogger(name, gz.ReadStdLogEnvVar(), verbosity)
	return logger, gz.NewContextWithLogger(context.Background(), logger)
}

// QueryCache connects the memcached query cache (GZ_FUEL_MEMCACHED_ADDR)
// and empties it; queries repopulate it.
func QueryCache(logger gz.Logger, isTest bool) {
	addr, err := gz.ReadEnvVar("GZ_FUEL_MEMCACHED_ADDR")
	if err != nil && !isTest {
		logger.Info("Missing GZ_FUEL_MEMCACHED_ADDR env variable. Memcached will not be available.")
	}
	globals.QueryCache = memcache.New(addr)
	if err := globals.QueryCache.DeleteAll(); err != nil {
		logger.Error("Failed to clear the memory cache.")
	}
}

// ResourceDir sets the root resource directory (IGN_FUEL_RESOURCE_DIR), or
// a fresh temporary one under `go test`.
func ResourceDir(isTest bool) {
	var err error
	if globals.ResourceDir, err = gz.ReadEnvVar("IGN_FUEL_RESOURCE_DIR"); err != nil && !isTest {
		log.Fatal("Missing IGN_FUEL_RESOURCE_DIR env variable. Resources will not be available. Quitting.")
	}
	if isTest {
		globals.ResourceDir, err = os.MkdirTemp("", "fuel-")
		if err != nil {
			log.Fatal("Could not initialize test globals.ResourceDir. Resources will not be available")
		}
	}
}

// Server connects the database and sets the JWT key (globals.Server).
func Server(logger gz.Logger) {
	key, err := gz.ReadEnvVar("AUTH0_RSA256_PUBLIC_KEY")
	if err != nil {
		logger.Info("Missing AUTH0_RSA256_PUBLIC_KEY env variable. Authentication will not work.")
	}
	globals.Server, err = gz.Init(key, "", nil)
	if err != nil {
		log.Fatal("Failed to initialize web server:", err)
	}
}

// VCS uses go-git for resource repositories.
func VCS() {
	globals.VCSRepoFactory = func(ctx context.Context, dirpath string) vcs.VCS {
		return vcs.GoGitVCS{}.NewRepo(dirpath)
	}
}

// Permissions loads the role-based permissions (IGN_FUEL_SYSTEM_ADMIN).
func Permissions(logger gz.Logger, isTest bool) error {
	var sysAdmin string
	if isTest {
		sysAdmin = SysAdminForTest
	} else {
		sysAdmin, _ = gz.ReadEnvVar("IGN_FUEL_SYSTEM_ADMIN")
	}
	if sysAdmin == "" {
		logger.Info("No IGN_FUEL_SYSTEM_ADMIN enivironment variable set. " +
			"No system administrator role will be created")
	}
	globals.Permissions = &permissions.Permissions{}
	return globals.Permissions.Init(globals.Server.Db, sysAdmin)
}

// S3 sets up the S3 client and bucket: an in-process fake under `go test`
// or with IGN_DEV_USE_FAKE_S3=true, else the real S3_BUCKET.
func S3(logger gz.Logger, isTest bool) {
	if isTest {
		globals.BucketS3 = "fuel-test"
		fakeS3("YOUR-ACCESSKEYID", "YOUR-SECRETACCESSKEY", "error creating test bucket:")
		return
	}
	// IGN_DEV_USE_FAKE_S3=true spins up an in-process gofakes3 server
	// so local development doesn't require real AWS credentials. This
	// mirrors the setup used by the Go test harness above.
	useFakeS3, _ := strconv.ParseBool(os.Getenv("IGN_DEV_USE_FAKE_S3"))
	if useFakeS3 {
		logger.Info("IGN_DEV_USE_FAKE_S3 enabled: using in-process gofakes3")
		globals.BucketS3 = "fuel-dev"
		fakeS3("DEV-ACCESSKEYID", "DEV-SECRETACCESSKEY", "error creating dev bucket: ")
		subt.BucketServerImpl = subt.NewS3Bucket(globals.BucketS3)
		return
	}
	p, err := gz.ReadEnvVar("S3_BUCKET")
	if err != nil {
		panic("error reading S3_BUCKET env var (set S3_BUCKET=<bucket> or IGN_DEV_USE_FAKE_S3=true)")
	}
	globals.SessionS3 = session.Must(session.NewSession())
	globals.S3 = s3.New(globals.SessionS3)
	globals.UploaderS3 = s3manager.NewUploader(globals.SessionS3)
	globals.BucketS3 = p
	subt.BucketServerImpl = subt.NewS3Bucket(p)
}

func fakeS3(id, secret, panicMsg string) {
	globals.HTTPTestS3Server = httptest.NewServer(gofakes3.New(s3mem.New()).Server())
	cfg := aws.Config{
		Credentials:      credentials.NewStaticCredentials(id, secret, ""),
		Endpoint:         aws.String(globals.HTTPTestS3Server.URL),
		Region:           aws.String("us-east-1"),
		DisableSSL:       aws.Bool(true),
		S3ForcePathStyle: aws.Bool(true),
	}
	globals.SessionS3 = session.Must(session.NewSession(&cfg))
	globals.S3 = s3.New(globals.SessionS3)
	globals.UploaderS3 = s3manager.NewUploader(globals.SessionS3)
	if _, err := globals.S3.CreateBucket(&s3.CreateBucketInput{Bucket: gz.String(globals.BucketS3)}); err != nil {
		panic(panicMsg + err.Error())
	}
}

// Storage is the file storage over the S3 client.
func Storage() {
	globals.Storage = storage.NewS3v1(globals.S3, globals.UploaderS3, globals.BucketS3)
}

// Shared runs every step for a process that serves no Fuel routes and
// runs no migrations: the AI Builder agent. The web server migrates the
// shared tables.
func Shared(name string) (gz.Logger, context.Context) {
	logger, ctx := Logger(name)
	isTest := IsGoTest()
	QueryCache(logger, isTest)
	ResourceDir(isTest)
	Server(logger)
	VCS()
	if err := Permissions(logger, isTest); err != nil {
		log.Fatal("Failed to load permissions: ", err)
	}
	S3(logger, isTest)
	Storage()
	return logger, ctx
}
