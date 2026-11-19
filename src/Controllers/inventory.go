package Controllers

import (
	"cuento-backend/src/Entities"
	"cuento-backend/src/Middlewares"
	"cuento-backend/src/Services"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type CreateInventoryItemRequest struct {
	ArcID        int     `json:"arc_id" binding:"required"`
	Name         string  `json:"name" binding:"required"`
	Description  *string `json:"description"`
	Icon         *string `json:"icon"`
	IsHidden     bool    `json:"is_hidden"`
	DisplayOrder int     `json:"display_order"`
}

type UpdateInventoryItemRequest struct {
	Name         *string `json:"name"`
	Description  *string `json:"description"`
	Icon         *string `json:"icon"`
	IsHidden     *bool   `json:"is_hidden"`
	DisplayOrder *int    `json:"display_order"`
}

type UpdateSceneInventoryRequest struct {
	Items []Services.InventoryQuantity `json:"items"`
}

type SetSceneInventoryRequest struct {
	UseInventory *bool `json:"use_inventory" binding:"required"`
}

type ArcInventoryResponse struct {
	Enabled bool                     `json:"enabled"`
	InUse   bool                     `json:"in_use"`
	CanEdit bool                     `json:"can_edit"`
	Items   []Entities.InventoryItem `json:"items"`
}

type SceneInventoriesResponse struct {
	EpisodeID   int                       `json:"episode_id"`
	TopicID     int64                     `json:"topic_id"`
	Name        string                    `json:"name"`
	Status      Entities.EpisodeStatus    `json:"status"`
	ArcID       int                       `json:"arc_id"`
	Catalogue   []Entities.InventoryItem  `json:"catalogue"`
	Inventories []Entities.SceneInventory `json:"inventories"`
}

type PostInventoryResponse struct {
	ProfileID int                       `json:"profile_id"`
	Name      string                    `json:"name"`
	Items     []Entities.InventoryEntry `json:"items"`
}

const (
	maxInventoryNameLength = 255
	maxInventoryIconLength = 512
)

func abortWith(c *gin.Context, code int, message string) {
	_ = c.Error(&Middlewares.AppError{Code: code, Message: message})
	c.Abort()
}

// abortInventoryError maps inventory service errors to HTTP responses.
func abortInventoryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		abortWith(c, http.StatusNotFound, "Not found")
	case errors.Is(err, Services.ErrInvalidInventoryItems), errors.Is(err, Services.ErrProfileNotInScene):
		abortWith(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, Services.ErrInventoryNotEnabled), errors.Is(err, Services.ErrInventoryInUse),
		errors.Is(err, Services.ErrInventoryItemInUse), errors.Is(err, Services.ErrInventoryLocked),
		errors.Is(err, Services.ErrSceneHasLockedInv), errors.Is(err, Services.ErrSceneNotUsingInventory),
		errors.Is(err, Services.ErrScenePending), errors.Is(err, Services.ErrSceneNotPending):
		abortWith(c, http.StatusConflict, err.Error())
	default:
		abortWith(c, http.StatusInternalServerError, "Inventory operation failed: "+err.Error())
	}
}

func intParam(c *gin.Context, name string) (int, bool) {
	v, err := strconv.Atoi(c.Param(name))
	if err != nil {
		abortWith(c, http.StatusBadRequest, "Invalid "+name)
		return 0, false
	}
	return v, true
}

// arcVisible reports whether the arc exists and the caller may see it (non-public arcs need a login).
func arcVisible(c *gin.Context, db *sql.DB, arcID, userID int) bool {
	var isPublic bool
	if err := db.QueryRow("SELECT is_public FROM arcs WHERE id = ?", arcID).Scan(&isPublic); err != nil {
		if err == sql.ErrNoRows {
			abortWith(c, http.StatusNotFound, "Arc not found")
		} else {
			abortWith(c, http.StatusInternalServerError, "Failed to fetch arc: "+err.Error())
		}
		return false
	}
	if !isPublic && userID == 0 {
		abortWith(c, http.StatusForbidden, "This arc is not public")
		return false
	}
	return true
}

func validateInventoryItemFields(name *string, icon *string) string {
	if name != nil && (strings.TrimSpace(*name) == "" || len(*name) > maxInventoryNameLength) {
		return "Name must be between 1 and 255 characters"
	}
	if icon != nil && len(*icon) > maxInventoryIconLength {
		return "Icon URL is too long"
	}
	return ""
}

func GetArcInventory(c *gin.Context, db *sql.DB) {
	arcID, ok := intParam(c, "id")
	if !ok {
		return
	}
	userID := Services.GetUserIdFromContext(c)
	if !arcVisible(c, db, arcID, userID) {
		return
	}

	resp := ArcInventoryResponse{CanEdit: userID != 0 && canEditArc(userID, arcID, db)}
	var err error
	if resp.Enabled, resp.InUse, err = Services.ArcInventoryState(db, arcID); err != nil {
		abortInventoryError(c, err)
		return
	}
	if resp.Items, err = Services.ListArcInventoryItems(db, arcID, resp.CanEdit); err != nil {
		abortInventoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func SetArcInventory(c *gin.Context, db *sql.DB, enable bool) {
	arcID, ok := intParam(c, "id")
	if !ok {
		return
	}
	userID := Services.GetUserIdFromContext(c)
	if !arcVisible(c, db, arcID, userID) {
		return
	}
	if !canEditArc(userID, arcID, db) {
		abortWith(c, http.StatusForbidden, "Only the game masters of this arc can change its inventory")
		return
	}

	var err error
	if enable {
		err = Services.EnableArcInventory(db, arcID)
	} else {
		err = Services.DisableArcInventory(db, arcID)
	}
	if err != nil {
		abortInventoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": enable})
}

func GetInventoryItem(c *gin.Context, db *sql.DB) {
	id, ok := intParam(c, "id")
	if !ok {
		return
	}
	item, err := Services.GetInventoryItem(db, id)
	if err != nil {
		abortInventoryError(c, err)
		return
	}
	userID := Services.GetUserIdFromContext(c)
	if !arcVisible(c, db, item.ArcID, userID) {
		return
	}
	canEdit := userID != 0 && canEditArc(userID, item.ArcID, db)
	if item.IsHidden && !canEdit {
		abortWith(c, http.StatusNotFound, "Item not found")
		return
	}
	item.CanEdit = &canEdit
	c.JSON(http.StatusOK, item)
}

func CreateInventoryItem(c *gin.Context, db *sql.DB) {
	var req CreateInventoryItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	if msg := validateInventoryItemFields(&req.Name, req.Icon); msg != "" {
		abortWith(c, http.StatusBadRequest, msg)
		return
	}
	if !canEditArc(Services.GetUserIdFromContext(c), req.ArcID, db) {
		abortWith(c, http.StatusForbidden, "Only the game masters of this arc can add items")
		return
	}

	id, err := Services.CreateInventoryItem(db, Entities.InventoryItem{
		ArcID: req.ArcID, Name: strings.TrimSpace(req.Name), Description: req.Description,
		Icon: req.Icon, IsHidden: req.IsHidden, DisplayOrder: req.DisplayOrder,
	})
	if err != nil {
		abortInventoryError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// itemForGM loads an item from the :id param and checks the caller is a GM of its arc.
func itemForGM(c *gin.Context, db *sql.DB) (Entities.InventoryItem, bool) {
	id, ok := intParam(c, "id")
	if !ok {
		return Entities.InventoryItem{}, false
	}
	item, err := Services.GetInventoryItem(db, id)
	if err != nil {
		abortInventoryError(c, err)
		return item, false
	}
	if !canEditArc(Services.GetUserIdFromContext(c), item.ArcID, db) {
		abortWith(c, http.StatusForbidden, "Only the game masters of this arc can change its items")
		return item, false
	}
	return item, true
}

func UpdateInventoryItem(c *gin.Context, db *sql.DB) {
	var req UpdateInventoryItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	if msg := validateInventoryItemFields(req.Name, req.Icon); msg != "" {
		abortWith(c, http.StatusBadRequest, msg)
		return
	}
	item, ok := itemForGM(c, db)
	if !ok {
		return
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		req.Name = &trimmed
	}
	err := Services.UpdateInventoryItem(db, item.ID, Services.InventoryItemUpdate{
		Name: req.Name, Description: req.Description, Icon: req.Icon, IsHidden: req.IsHidden, DisplayOrder: req.DisplayOrder,
	})
	if err != nil {
		abortInventoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Item updated"})
}

func DeleteInventoryItem(c *gin.Context, db *sql.DB) {
	item, ok := itemForGM(c, db)
	if !ok {
		return
	}
	if err := Services.DeleteInventoryItem(db, item.ID); err != nil {
		abortInventoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Item deleted"})
}

// sceneForReader loads the scene from the :id param and checks the caller can read its subforum.
func sceneForReader(c *gin.Context, db *sql.DB) (Services.SceneInventoryContext, bool) {
	id, ok := intParam(c, "id")
	if !ok {
		return Services.SceneInventoryContext{}, false
	}
	ctx, err := Services.GetSceneInventoryContext(db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			abortWith(c, http.StatusNotFound, "Scene not found")
		} else {
			abortInventoryError(c, err)
		}
		return ctx, false
	}
	userID := Services.GetUserIdFromContext(c)
	if hasPerm, err := Services.HasPermission(userID, fmt.Sprintf("subforum_read:%d", ctx.SubforumID), db); err != nil || !hasPerm {
		abortWith(c, http.StatusForbidden, "You don't have access to this scene")
		return ctx, false
	}
	return ctx, true
}

// sceneForGM is sceneForReader plus: the caller is a GM of the scene's arc.
func sceneForGM(c *gin.Context, db *sql.DB) (Services.SceneInventoryContext, bool) {
	ctx, ok := sceneForReader(c, db)
	if !ok {
		return ctx, false
	}
	if ctx.ArcID == 0 || !canEditArc(Services.GetUserIdFromContext(c), ctx.ArcID, db) {
		abortWith(c, http.StatusForbidden, "Only the game masters of this scene's arc can manage inventories")
		return ctx, false
	}
	return ctx, true
}

// sceneUsingInventory is sceneForGM plus: the scene uses inventory.
func sceneUsingInventory(c *gin.Context, db *sql.DB) (Services.SceneInventoryContext, bool) {
	ctx, ok := sceneForGM(c, db)
	if ok && !ctx.UsesInventory {
		abortInventoryError(c, Services.ErrSceneNotUsingInventory)
		return ctx, false
	}
	return ctx, ok
}

func SetSceneInventory(c *gin.Context, db *sql.DB) {
	var req SetSceneInventoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	ctx, ok := sceneForGM(c, db)
	if !ok {
		return
	}
	if err := Services.SetSceneUsesInventory(db, ctx, *req.UseInventory); err != nil {
		abortInventoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"use_inventory": *req.UseInventory})
}

func GetSceneInventories(c *gin.Context, db *sql.DB) {
	ctx, ok := sceneUsingInventory(c, db)
	if !ok {
		return
	}
	inventories, err := Services.GetSceneInventories(db, ctx.EpisodeID)
	if err != nil {
		abortInventoryError(c, err)
		return
	}
	Services.LocalizeSceneInventories(inventories, Services.GetUserLocaleFromContext(c))
	catalogue, err := Services.ListArcInventoryItems(db, ctx.ArcID, true)
	if err != nil {
		abortInventoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, SceneInventoriesResponse{
		EpisodeID: ctx.EpisodeID, TopicID: ctx.TopicID, Name: ctx.Name, Status: ctx.Status, ArcID: ctx.ArcID,
		Catalogue: catalogue, Inventories: inventories,
	})
}

func UpdateSceneInventory(c *gin.Context, db *sql.DB) {
	var req UpdateSceneInventoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	profileID, ok := intParam(c, "profile_id")
	if !ok {
		return
	}
	ctx, ok := sceneUsingInventory(c, db)
	if !ok {
		return
	}
	if err := Services.SetSceneInventoryItems(db, ctx, profileID, req.Items); err != nil {
		abortInventoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Inventory saved"})
}

func LockSceneInventory(c *gin.Context, db *sql.DB) {
	profileID, ok := intParam(c, "profile_id")
	if !ok {
		return
	}
	ctx, ok := sceneUsingInventory(c, db)
	if !ok {
		return
	}
	if err := Services.LockSceneInventory(db, ctx, profileID, Services.GetUserIdFromContext(c)); err != nil {
		abortInventoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Inventory locked"})
}

func StartScene(c *gin.Context, db *sql.DB) {
	ctx, ok := sceneUsingInventory(c, db)
	if !ok {
		return
	}
	unlocked, err := Services.StartScene(db, ctx, Services.GetUserIdFromContext(c))
	if errors.Is(err, Services.ErrInventoriesUnlocked) {
		c.JSON(http.StatusConflict, gin.H{"error": "Lock every inventory before starting the scene", "unlocked": unlocked})
		return
	}
	if err != nil {
		abortInventoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"episode_status": Entities.ActiveEpisode})
}

// GetCurrentSceneInventories returns the latest inventories the caller may see in a scene:
// GMs see every profile, players only their own characters and masks.
func GetCurrentSceneInventories(c *gin.Context, db *sql.DB) {
	ctx, ok := sceneForReader(c, db)
	if !ok {
		return
	}
	if !ctx.UsesInventory {
		abortInventoryError(c, Services.ErrSceneNotUsingInventory)
		return
	}
	list, err := Services.GetCurrentSceneInventories(db, ctx.EpisodeID)
	if err != nil {
		abortInventoryError(c, err)
		return
	}
	userID := Services.GetUserIdFromContext(c)
	if ctx.ArcID == 0 || !canEditArc(userID, ctx.ArcID, db) {
		own := []Entities.SceneInventory{}
		for _, inv := range list {
			if inv.OwnerUserID == userID {
				own = append(own, inv)
			}
		}
		list = own
	}
	Services.LocalizeSceneInventories(list, Services.GetUserLocaleFromContext(c))
	c.JSON(http.StatusOK, list)
}

// GetPostInventory returns the inventory copy of a post, for the post's player and the arc's GMs only.
func GetPostInventory(c *gin.Context, db *sql.DB) {
	postID, ok := intParam(c, "id")
	if !ok {
		return
	}
	ctx, err := Services.GetPostInventoryContext(db, postID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			abortWith(c, http.StatusNotFound, "This post has no character inventory")
		} else {
			abortInventoryError(c, err)
		}
		return
	}
	userID := Services.GetUserIdFromContext(c)
	if hasPerm, err := Services.HasPermission(userID, fmt.Sprintf("subforum_read:%d", ctx.Scene.SubforumID), db); err != nil || !hasPerm {
		abortWith(c, http.StatusForbidden, "You don't have access to this scene")
		return
	}
	if !ctx.Scene.UsesInventory {
		abortInventoryError(c, Services.ErrSceneNotUsingInventory)
		return
	}
	if ctx.OwnerUserID != userID && (ctx.Scene.ArcID == 0 || !canEditArc(userID, ctx.Scene.ArcID, db)) {
		abortWith(c, http.StatusForbidden, "Only the player and the game masters can see this inventory")
		return
	}
	items, err := Services.GetPostInventoryItems(db, postID)
	if err != nil {
		abortInventoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, PostInventoryResponse{ProfileID: ctx.ProfileID, Name: ctx.Name, Items: items})
}
