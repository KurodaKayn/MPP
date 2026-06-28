package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *AIContextSnapshot) BeforeCreate(tx *gorm.DB) (err error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	if s.WorkspaceID == uuid.Nil {
		s.WorkspaceID = deriveWorkspaceIDFromProject(tx, s.ProjectID, s.CreatedByID)
	}
	return
}

func (r *AIGrowthOptimizationRun) BeforeCreate(tx *gorm.DB) (err error) {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	if r.WorkspaceID == uuid.Nil {
		r.WorkspaceID = deriveWorkspaceIDFromProject(tx, r.ProjectID, r.CreatedByID)
	}
	return
}

func (p *AIProposal) BeforeCreate(tx *gorm.DB) (err error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	if p.WorkspaceID == uuid.Nil {
		p.WorkspaceID = deriveWorkspaceIDFromProject(tx, p.ProjectID, uuid.Nil)
	}
	return
}

func (s *AIDraftingSession) BeforeCreate(tx *gorm.DB) (err error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	if s.WorkspaceID == uuid.Nil {
		s.WorkspaceID = deriveWorkspaceIDFromProject(tx, s.ProjectID, s.CreatedByID)
	}
	return
}
