package subt

import (
	"bitbucket.org/ignitionrobotics/ign-fuelserver/bundles/users"
	"bitbucket.org/ignitionrobotics/ign-go"
	"context"
	"github.com/jinzhu/gorm"
	"log"
)

// Initialize initializes the SubT bundle
func Initialize(ctx context.Context, db *gorm.DB) {
	tx := db.Begin()
	// First create subtuser
	subtuser := "subtdefault"
	if hasTable := db.HasTable(&users.User{}); hasTable {
		var usr users.User
		// Get the user with the requested username
		db.Unscoped().Where("username = ?", subtuser).First(&usr)
		if usr.Username == nil {
			// Create the user, so no one else can create it
			username := subtuser
			name := "The SubT default user"
			email := "anonymous@osrf.org"
			identity := "test-subt-identity"
			org := "OSRF"
			u := users.User{
				Name:         &name,
				Username:     &username,
				Email:        &email,
				Identity:     &identity,
				Organization: &org}
			if _, err := users.CreateUser(ctx, db, &u, false); err != nil {
				ign.LoggerFromContext(ctx).Error("Error creating subt default user", err)
				log.Fatal("Error creating subt default user", err)
			}
		}
	}

	var ownerDbUser users.User
	tx.Where("username = ?", subtuser).First(&ownerDbUser)

	var uo users.UniqueOwner
	db.Unscoped().Where("name = ?", SubTPortalName).First(&uo)
	if uo.Name == nil {
		org := users.CreateOrganization{Name: SubTPortalName}
		if _, em := (&users.OrganizationService{}).CreateOrganization(ctx, tx,
			org, &ownerDbUser); em != nil {
			log.Fatal("[SubT] Error trying to create Organization", em.BaseError)
		}
	}

	if err := tx.Commit().Error; err != nil {
		log.Fatal("[SubT] Failed during initialization")
	}
	log.Println("[SubT] Successfully initialized")
}
