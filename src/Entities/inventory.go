package Entities

import "time"

// InventoryItem is a catalogue entry of an arc: an example of a physical item, not a count.
type InventoryItem struct {
	ID           int     `json:"id"`
	ArcID        int     `json:"arc_id"`
	Name         string  `json:"name"`
	Description  *string `json:"description"`
	Icon         *string `json:"icon"`
	IsHidden     bool    `json:"is_hidden"`
	DisplayOrder int     `json:"display_order"`
	CanEdit      *bool   `json:"can_edit,omitempty"`
}

// InventoryEntry is one catalogue item held in some quantity.
type InventoryEntry struct {
	ItemID      int     `json:"item_id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Icon        *string `json:"icon"`
	Quantity    int     `json:"quantity"`
}

// SceneInventory is the inventory of one character profile (main or mask) in a scene.
type SceneInventory struct {
	ProfileID         int              `json:"profile_id"`
	Name              string           `json:"name"`
	IsMask            bool             `json:"is_mask"`
	OwnerUserID       int              `json:"owner_user_id"`
	OwnerUsername     string           `json:"owner_username"`
	Exists            bool             `json:"exists"`
	IsLocked          bool             `json:"is_locked"`
	LockedAt          *time.Time       `json:"-"`
	LockedAtLocalized string           `json:"locked_at,omitempty"`
	Items             []InventoryEntry `json:"items"`
}

// InventoryRecipient is a profile of a scene that can receive items: it has a locked inventory there.
type InventoryRecipient struct {
	ProfileID int    `json:"profile_id"`
	Name      string `json:"name"`
}
