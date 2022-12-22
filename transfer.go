package main

import (
	"fmt"
	"github.com/gazebo-web/fuel-server/bundles/common_resources"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/permissions"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"

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
func processTransferRequest(sourceOwner string, tx *gorm.DB, r *http.Request) (*TransferAsset, *gz.ErrMsg) {
	// TransferAsset is the input data
	var transferAsset TransferAsset
	if em := ParseStruct(&transferAsset, r, false); em != nil {
		return nil, em
	}

	// Step 1: check that the destination is an organization
	_, em := users.ByOrganizationName(tx, transferAsset.DestOwner, false)
	if em != nil {
		extra := fmt.Sprintf("Organization [%s] not found", transferAsset.DestOwner)
		return nil, gz.NewErrorMessageWithArgs(gz.ErrorNameNotFound, em.BaseError, []string{extra})
	}

	// Step 2: check write permissions of the requesting user
	if ok, em := globals.Permissions.IsAuthorized(sourceOwner,
		transferAsset.DestOwner, permissions.Write); !ok {
		extra := fmt.Sprintf("User [%s] is not authorized", sourceOwner)
		return nil, gz.NewErrorMessageWithArgs(gz.ErrorUnauthorized, em.BaseError, []string{extra})
	}

	return &transferAsset, nil
}

// transferMoveResource will move an resource, such as a model, world, or collection,
// from a user to an organization.
func transferMoveResource(tx *gorm.DB, resource commonres.Resource, sourceOwner, destOwner string) *gz.ErrMsg {

	// Attempt to move the resource
	if em := commonres.MoveResource(resource, destOwner); em != nil {
		return em
	}

	// Add permissions to destination owner
	_, err := globals.Permissions.AddPermission(destOwner, *resource.GetUUID(), permissions.Read)
	if err != nil {
		// Revert move
		commonres.MoveResource(resource, sourceOwner)
		return gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	_, err = globals.Permissions.AddPermission(destOwner, *resource.GetUUID(), permissions.Write)
	if err != nil {
		// Revert move
		commonres.MoveResource(resource, sourceOwner)
		return gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	// Remove permissions from original owner
	_, err = globals.Permissions.RemovePermission(sourceOwner, *resource.GetUUID(), permissions.Read)
	if err != nil {
		// Revert move
		commonres.MoveResource(resource, sourceOwner)
		return gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	_, err = globals.Permissions.RemovePermission(sourceOwner, *resource.GetUUID(), permissions.Write)
	if err != nil {
		// Revert move
		commonres.MoveResource(resource, sourceOwner)
		return gz.NewErrorMessageWithBase(gz.ErrorUnexpected, err)
	}

	return nil
}
