package models

type Series struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IPID        uint   `json:"ip_id"` // Phase 2: link to IPProfile (not yet a model)
}

func (Series) TableName() string { return "series" }
