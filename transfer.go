package main

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/gazebo-web/fuel-server/bundles/common_resources"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/permissions"
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

// transferMoveResource will move an resource, such as a model, world, or collection,
// from a user to an organization.
func transferMoveResource(tx *gorm.DB, resource commonres.Resource, sourceOwner, destOwner string) *ign.ErrMsg {

	// Attempt to move the resource
	if em := commonres.MoveResource(resource, destOwner); em != nil {
		return em
	}

	// Add permissions to destination owner
	_, err := globals.Permissions.AddPermission(destOwner, *resource.GetUUID(), permissions.Read)
	if err != nil {
		// Revert move
		commonres.MoveResource(resource, sourceOwner)
		return ign.NewErrorMessageWithBase(ign.ErrorUnexpected, err)
	}

	_, err = globals.Permissions.AddPermission(destOwner, *resource.GetUUID(), permissions.Write)
	if err != nil {
		// Revert move
		commonres.MoveResource(resource, sourceOwner)
		return ign.NewErrorMessageWithBase(ign.ErrorUnexpected, err)
	}

	// Remove permissions from original owner
	_, err = globals.Permissions.RemovePermission(sourceOwner, *resource.GetUUID(), permissions.Read)
	if err != nil {
		// Revert move
		commonres.MoveResource(resource, sourceOwner)
		return ign.NewErrorMessageWithBase(ign.ErrorUnexpected, err)
	}

	_, err = globals.Permissions.RemovePermission(sourceOwner, *resource.GetUUID(), permissions.Write)
	if err != nil {
		// Revert move
		commonres.MoveResource(resource, sourceOwner)
		return ign.NewErrorMessageWithBase(ign.ErrorUnexpected, err)
	}

	return nil
}
