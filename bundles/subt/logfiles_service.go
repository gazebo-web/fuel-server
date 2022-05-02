package subt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/satori/go.uuid"
	res "github.com/gazebo-web/fuel-server/bundles/common_resources"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/globals"
	p "github.com/gazebo-web/fuel-server/permissions"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"io"
	"time"
)

// LogService is the main struct exported by this service.
type LogService struct{}

// BucketServerImpl holds the bucket server to be used.
var BucketServerImpl BucketServer

// BucketServer is an interface to be followed by Bucket server implementations.
// Eg. S3
type BucketServer interface {
	GetBucketName(bucket string) string
	Upload(ctx context.Context, f io.Reader, bucket, fPath string) (*string, error)
	RemoveFile(ctx context.Context, bucket, fPath string) error
	GetPresignedURL(ctx context.Context, bucket, fPath string) (*string, error)
}

func (s *LogService) getLogFileFromDB(tx *gorm.DB, id uint) (*LogFile, *ign.ErrMsg) {
	var lf LogFile
	if err := tx.First(&lf, id).Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorNameNotFound, err)
	}
	return &lf, nil
}

// CreateLog submits a new (pending) log file.
// user argument is the active user requesting the operation.
func (s *LogService) CreateLog(ctx context.Context, tx *gorm.DB, f io.Reader,
	filename, comp string, ls *LogSubmission, user *users.User) (*LogFile, *ign.ErrMsg) {

	// Verify and set the owner
	owner := ls.Owner
	if owner == "" {
		owner = *user.Username
	} else {
		if ok, em := users.VerifyOwner(tx, owner, *user.Username, p.Read); !ok {
			return nil, em
		}
	}
	// Sanity check: make sure the owner is a participant in the competition
	if _, em := getParticipant(tx, comp, owner); em != nil {
		return nil, em
	}

	// Create a new uuid
	uuidStr := uuid.NewV4().String()
	private := true
	if ls.Private != nil {
		private = *ls.Private
	}

	pending := int(StForReview)
	log := LogFile{UUID: &uuidStr, Owner: &owner,
		Creator: user.Username, Private: &private, Competition: &comp,
		Comments: &ls.Description, Status: &pending,
	}
	if err := tx.Create(&log).Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	path := fmt.Sprintf("%s/%d-%s", owner, log.ID, filename)
	// Set the created path to DB record
	tx.Model(&log).Update("Location", &path)

	_, err := BucketServerImpl.Upload(ctx, f, comp, path)
	if err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbSave, err)
	}

	// Set read permissions to owner (eg, the team) as well as Competition
	// organizing team (SubT).
	// The Write permission will be only for admins of Competition.
	lfName := log.name()
	if _, em := globals.Permissions.AddPermission(comp, lfName, p.Read); em != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorUnexpected, em)
	}
	if _, em := globals.Permissions.AddPermission(owner, lfName, p.Read); em != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorUnexpected, em)
	}

	sendMail(fmt.Sprintf("LogFile uploaded by participant [%s]", owner), &log)
	return &log, nil
}

func sendMail(subject string, objsToMarshal ...interface{}) *ign.ErrMsg {
	sender := globals.FlagsEmailSender
	recipient := globals.FlagsEmailRecipient
	if sender == "" || recipient == "" {
		// don't send email. Just return
		return nil
	}

	var bs bytes.Buffer
	for _, o := range objsToMarshal {
		b, err := json.MarshalIndent(o, "", "  ")
		if err != nil {
			return ign.NewErrorMessageWithBase(ign.ErrorUnexpected, err)
		}
		bs.Write(b)
		bs.WriteString("\n\n")
	}

	// send email
	err := ign.SendEmail(sender, recipient, subject, bs.String())
	if err != nil {
		return ign.NewErrorMessageWithBase(ign.ErrorUnexpected, err)
	}

	return nil
}

// UpdateLogFile updates a log file. Eg. sets score and status.
// user argument is the active user requesting the operation.
func (s *LogService) UpdateLogFile(ctx context.Context, tx *gorm.DB, comp string,
	id uint, su *SubmissionUpdate, user *users.User) (*LogFile, *ign.ErrMsg) {

	// Only admins of the competition can update submissions for that competition.
	if ok, em := globals.Permissions.IsAuthorized(*user.Username, comp, p.Write); !ok {
		return nil, em
	}

	// Sanity check: make sure it is a pending submission for this competition
	log, em := s.getLogFileFromDB(tx, id)
	if em != nil {
		return nil, em
	}
	if *log.Competition != comp {
		return nil, ign.NewErrorMessage(ign.ErrorNameNotFound)
	}

	now := time.Now()
	up := tx.Model(log).Update("ResolvedAt", &now).
		Update("Status", iptr(int(su.Status))).
		Update("Score", su.Score)

	if su.Comments != nil {
		up.Update("Comments", su.Comments)
	}

	return log, nil
}

// GetLogFileForDownload returns an URL (for downloading) a log file.
// user argument is the active user requesting the operation.
func (s *LogService) GetLogFileForDownload(ctx context.Context, tx *gorm.DB,
	comp string, id uint, user *users.User) (*string, *ign.ErrMsg) {

	log, em := s.getLogFileFromDB(tx, id)
	if em != nil {
		return nil, em
	}

	if ok, em := globals.Permissions.IsAuthorized(*user.Username, log.name(), p.Read); !ok {
		return nil, em
	}

	url, err := BucketServerImpl.GetPresignedURL(ctx, comp, *log.Location)
	if err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorUnexpected, err)
	}

	return url, nil
}

// GetLogFile returns a single log file record.
// user argument is the active user requesting the operation.
func (s *LogService) GetLogFile(ctx context.Context, tx *gorm.DB,
	comp string, id uint, user *users.User) (*LogFile, *ign.ErrMsg) {

	log, em := s.getLogFileFromDB(tx, id)
	if em != nil {
		return nil, em
	}

	if ok, em := globals.Permissions.IsAuthorized(*user.Username, log.name(), p.Read); !ok {
		return nil, em
	}

	return log, nil
}

// RemoveLogFile removes a log file.
// user argument is the active user requesting the operation.
// Returns the removed log file
func (s *LogService) RemoveLogFile(ctx context.Context, tx *gorm.DB, comp string,
	id uint, user *users.User) (*LogFile, *ign.ErrMsg) {

	// Only system admins can delete log files
	if ok := globals.Permissions.IsSystemAdmin(*user.Username); !ok {
		return nil, ign.NewErrorMessage(ign.ErrorUnauthorized)
	}

	log, em := s.getLogFileFromDB(tx, id)
	if em != nil {
		return nil, em
	}
	if *log.Competition != comp {
		return nil, ign.NewErrorMessage(ign.ErrorNameNotFound)
	}

	err := BucketServerImpl.RemoveFile(ctx, comp, *log.Location)
	if err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorUnexpected, err)
	}

	// if successfully removed from bucket server, then remove from DB
	if err := tx.Delete(log).Error; err != nil {
		return nil, ign.NewErrorMessageWithBase(ign.ErrorDbDelete, err)
	}

	return log, nil
}

// LogFileList returns a list of paginated log files.
// Members of the submitting team can see the list of log files they submitted.
// Members of the organizing group (eg. SubT) can see all log files.
func (s *LogService) LogFileList(pr *ign.PaginationRequest, tx *gorm.DB, comp string,
	owner *string, status SubmissionStatus, reqUser *users.User) (*LogFiles, *ign.PaginationResult, *ign.ErrMsg) {

	// Create the DB query
	q := tx.Model(&LogFile{}).Order("id desc", true)
	q = q.Where("status = ? AND competition = ?", int(status), comp)

	// If reqUser belongs to the main competition group, then can see all log files.
	// Otherwise, only those log files the reqUser's team submitted.
	if ok, _ := globals.Permissions.IsAuthorized(*reqUser.Username, comp, p.Read); !ok {
		// filter resources based on privacy setting
		q = res.QueryForResourceVisibility(tx, q, owner, reqUser)
	}

	var logs LogFiles
	pagination, err := ign.PaginateQuery(q, &logs, *pr)
	if err != nil {
		return nil, nil, ign.NewErrorMessageWithBase(ign.ErrorInvalidPaginationRequest, err)
	}
	if !pagination.PageFound {
		return nil, nil, ign.NewErrorMessage(ign.ErrorPaginationPageNotFound)
	}

	return &logs, pagination, nil
}
