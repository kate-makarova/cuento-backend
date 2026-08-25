package Entities

import "time"

type CampaignStatus int

const (
	PendingCampaign  CampaignStatus = 0
	ActiveCampaign   CampaignStatus = 1
	FinishedCampaign CampaignStatus = 2
	ArchivedCampaign CampaignStatus = 3
)

type Campaign struct {
	Id          int            `json:"id" db:"id"`
	Title       string         `json:"title" db:"title"`
	Summary     string         `json:"summary" db:"summary"`
	Status      CampaignStatus `json:"status" db:"status"`
	DateCreated time.Time      `json:"date_created" db:"date_created"`
	StartDate   *time.Time     `json:"start_date" db:"start_date"`
	EndDate     *time.Time     `json:"end_date" db:"end_date"`
	Characters    []ShortCharacter `json:"characters" db:"-"`
	Episodes      []Episode        `json:"episodes" db:"-"`
	NpcCharacters []NpcCharacter   `json:"npc_characters" db:"-"`
}
