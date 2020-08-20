package main

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/common_resources"
	"gitlab.com/ignitionrobotics/web/fuelserver/bundles/users"
	"gitlab.com/ignitionrobotics/web/fuelserver/globals"
	"gitlab.com/ignitionrobotics/web/fuelserver/permissions"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"net/http"
)

// TransferAsset encapsulates data required to transfer an asset, such as a
// model, world, or collection
type TransferAsset struct {
	// Receiver of the asset.
	DestOwner string `json:"destOwner"`
}

// processTransferRequest reads the request data into a TransferAsset struct,
// and checks permissions. A pointer to the TransferAsset is returned, or
// nil on error.
func processTransferRequest(sourceOwner string, tx *gorm.DB, r *http.Request) (*TransferAsset, *ign.ErrMsg) {
	// TransferAsset is the input data
	var transferAsset TransferAsset
	if em := ParseStruct(&transferAsset, r, false); em != nil {
		return nil, em
	}

	// Step 1: check that the destination is an organization
	_, em := users.ByOrganizationName(tx, transferAsset.DestOwner, false)
	if em != nil {
		extra := fmt.Sprintf("Organization [%s] not found", transferAsset.DestOwner)
		return nil, ign.NewErrorMessageWithArgs(ign.ErrorNameNotFound, em.BaseError, []string{extra})
	}

	// Step 2: check write permissions of the requesting user
	if ok, em := globals.Permissions.IsAuthorized(sourceOwner,
		transferAsset.DestOwner, permissions.Write); !ok {
		extra := fmt.Sprintf("User [%s] is not authorized", sourceOwner)
		return nil, ign.NewErrorMessageWithArgs(ign.ErrorUnauthorized, em.BaseError, []string{extra})
	}

	return &transferAsset, nil
}

// transferMoveAsset will move an asset, such as a model, world, or collection,
// from a user to an organization.
func transferMoveAsset(tx *gorm.DB, resource commonres.Resource, destOwner string) *ign.ErrMsg {

	// Attempt to move the asset
	newLocation, em := commonres.Move(resource, destOwner)

	if em != nil {
		return em
	}

	// Transfer the world to the new owner.
	tx.Model(&resource).Updates(map[string]interface{}{
            "Owner": destOwner,
            "Location": *newLocation,
        })

	return nil
}
