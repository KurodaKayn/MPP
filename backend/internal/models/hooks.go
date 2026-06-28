package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func (u *User) BeforeCreate(_ *gorm.DB) (err error) {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return
}

func (p *Project) BeforeCreate(_ *gorm.DB) (err error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return
}

func (m *MediaAsset) BeforeCreate(_ *gorm.DB) (err error) {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.Status == "" {
		m.Status = MediaAssetStatusPending
	}
	if m.LibraryScope == "" {
		m.LibraryScope = MediaAssetLibraryScopeProject
	}
	if m.Tags == nil {
		m.Tags = datatypes.JSON([]byte(`[]`))
	}
	return
}

func (t *ContentTemplate) BeforeCreate(_ *gorm.DB) (err error) {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	if t.Scope == "" {
		t.Scope = ContentTemplateScopePersonal
	}
	if t.DefaultPlatforms == nil {
		t.DefaultPlatforms = datatypes.JSON([]byte(`[]`))
	}
	if t.PlatformConfig == nil {
		t.PlatformConfig = datatypes.JSON([]byte(`{}`))
	}
	if t.Tags == nil {
		t.Tags = datatypes.JSON([]byte(`[]`))
	}
	return
}

func (b *BrandProfile) BeforeCreate(_ *gorm.DB) (err error) {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	if b.BannedWords == nil {
		b.BannedWords = datatypes.JSON([]byte(`[]`))
	}
	if b.DefaultTags == nil {
		b.DefaultTags = datatypes.JSON([]byte(`[]`))
	}
	return
}

func (u *MediaAssetUsage) BeforeCreate(_ *gorm.DB) (err error) {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return
}

func (p *ProjectPlatformPublication) BeforeCreate(tx *gorm.DB) (err error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	if p.WorkspaceID == uuid.Nil {
		p.WorkspaceID = deriveWorkspaceIDFromProject(tx, p.ProjectID, uuid.Nil)
	}
	if p.DraftStatus == "" {
		p.DraftStatus = PublicationDraftStatusUnsynced
	}
	if p.ReviewStatus == "" {
		p.ReviewStatus = PublicationReviewStatusDraft
	}
	return
}

func (e *PublishEvent) BeforeCreate(tx *gorm.DB) (err error) {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	if e.WorkspaceID == uuid.Nil {
		e.WorkspaceID = deriveWorkspaceIDFromProject(tx, e.ProjectID, e.UserID)
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	return
}

func (s *ScheduledPublication) BeforeCreate(_ *gorm.DB) (err error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	if s.Status == "" {
		s.Status = ScheduledPublicationStatusScheduled
	}
	if s.Timezone == "" {
		s.Timezone = "UTC"
	}
	return
}

func (a *PublishAttempt) BeforeCreate(_ *gorm.DB) (err error) {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.Status == "" {
		a.Status = PublishAttemptStatusRunning
	}
	return
}

func (e *OutboxEvent) BeforeCreate(_ *gorm.DB) (err error) {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	if e.Payload == nil {
		e.Payload = datatypes.JSON([]byte(`{}`))
	}
	if e.Status == "" {
		e.Status = OutboxStatusPending
	}
	return
}

func (w *Workspace) BeforeCreate(_ *gorm.DB) (err error) {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	if w.Status == "" {
		w.Status = WorkspaceStatusActive
	}
	return
}

func (a *WorkspaceActivity) BeforeCreate(_ *gorm.DB) (err error) {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	return
}

func (n *Notification) BeforeCreate(_ *gorm.DB) (err error) {
	if n.ID == uuid.Nil {
		n.ID = uuid.New()
	}
	if n.Status == "" {
		n.Status = NotificationStatusUnread
	}
	return
}

func (i *WorkspaceInvite) BeforeCreate(_ *gorm.DB) (err error) {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	if i.Status == "" {
		i.Status = WorkspaceInviteStatusPending
	}
	return
}

func (a *ProjectActivity) BeforeCreate(tx *gorm.DB) (err error) {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.WorkspaceID == uuid.Nil {
		a.WorkspaceID = deriveWorkspaceIDFromProject(tx, a.ProjectID, a.ActorUserID)
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	return
}

func (c *ProjectComment) BeforeCreate(tx *gorm.DB) (err error) {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	if c.WorkspaceID == uuid.Nil {
		c.WorkspaceID = deriveWorkspaceIDFromProject(tx, c.ProjectID, c.AuthorID)
	}
	if c.Status == "" {
		c.Status = ProjectCommentStatusOpen
	}
	return
}

func (v *ProjectVersion) BeforeCreate(tx *gorm.DB) (err error) {
	if v.ID == uuid.Nil {
		v.ID = uuid.New()
	}
	if v.WorkspaceID == uuid.Nil {
		v.WorkspaceID = deriveWorkspaceIDFromProject(tx, v.ProjectID, v.CreatedBy)
	}
	return
}

func (l *ProjectShareLink) BeforeCreate(tx *gorm.DB) (err error) {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	if l.WorkspaceID == uuid.Nil {
		l.WorkspaceID = deriveWorkspaceIDFromProject(tx, l.ProjectID, l.CreatedBy)
	}
	if l.Status == "" {
		l.Status = ProjectShareLinkStatusActive
	}
	return
}

func (pa *PlatformAccount) BeforeCreate(_ *gorm.DB) (err error) {
	if pa.ID == uuid.Nil {
		pa.ID = uuid.New()
	}
	if pa.OwnerUserID == nil && pa.UserID != uuid.Nil {
		ownerID := pa.UserID
		pa.OwnerUserID = &ownerID
	}
	if pa.ConnectedByUserID == nil && pa.UserID != uuid.Nil {
		connectedBy := pa.UserID
		pa.ConnectedByUserID = &connectedBy
	}
	if pa.WorkspaceID == nil && pa.UserID != uuid.Nil {
		workspaceID := PersonalWorkspaceID(pa.UserID)
		pa.WorkspaceID = &workspaceID
	}
	if pa.DisplayName == "" {
		pa.DisplayName = pa.Username
	}
	if pa.ShareScope == "" {
		pa.ShareScope = PlatformAccountSharePrivate
	}
	if pa.HealthStatus == "" {
		pa.HealthStatus = PlatformAccountHealthUnknown
	}
	if pa.CredentialSecretRef == "" {
		pa.CredentialSecretRef = "platform-account:" + pa.ID.String()
	}
	return
}

func (s *RemoteBrowserSession) BeforeCreate(_ *gorm.DB) (err error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	return
}

func (g *PlatformAccountGrant) BeforeCreate(_ *gorm.DB) (err error) {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	return
}

func (t *ExtensionCallbackToken) BeforeCreate(tx *gorm.DB) (err error) {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	if t.WorkspaceID == uuid.Nil {
		t.WorkspaceID = deriveWorkspaceIDFromProject(tx, t.ProjectID, t.UserID)
	}
	return
}

func (e *ExtensionExecutionEvent) BeforeCreate(tx *gorm.DB) (err error) {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	if e.WorkspaceID == uuid.Nil {
		e.WorkspaceID = deriveWorkspaceIDFromProject(tx, e.ProjectID, e.UserID)
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	return
}

func (c *ExtensionExecutionEventClaim) BeforeCreate(_ *gorm.DB) (err error) {
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	return
}
