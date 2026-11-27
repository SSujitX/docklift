package store

import (
	"context"
	"fmt"

	"docklift/internal/db"
)

// Deployment is a row of "deployments". ImageTags is the Json column holding
// a service name -> image tag map (null when unset).
type Deployment struct {
	ID            string   `json:"id"`
	ProjectID     string   `json:"project_id"`
	Status        *string  `json:"status"`
	Trigger       *string  `json:"trigger"`
	CommitMessage *string  `json:"commit_message"`
	CommitSHA     *string  `json:"commit_sha"`
	ImageTags     *db.JSON `json:"image_tags"`
	Logs          *string  `json:"logs"`
	CreatedAt     *db.Time `json:"created_at"`
	FinishedAt    *db.Time `json:"finished_at"`
}

const deploymentCols = `"id", "project_id", "status", "trigger", "commit_message", "commit_sha", "image_tags", "logs", "created_at", "finished_at"`

func scanDeployment(r rowScanner) (*Deployment, error) {
	var d Deployment
	if err := r.Scan(&d.ID, &d.ProjectID, &d.Status, &d.Trigger, &d.CommitMessage, &d.CommitSHA, &d.ImageTags,
		&d.Logs, &d.CreatedAt, &d.FinishedAt); err != nil {
		return nil, err
	}
	return &d, nil
}

// DeploymentFilter selects deployments; set fields are AND-ed.
type DeploymentFilter struct {
	ID        *string
	ProjectID *string
	Status    *string
	StatusNot *string // Prisma `not`; also excludes NULL status
	Trigger   *string
}

func (f DeploymentFilter) where() where {
	var w where
	if f.ID != nil {
		w.eq(`"id"`, *f.ID)
	}
	if f.ProjectID != nil {
		w.eq(`"project_id"`, *f.ProjectID)
	}
	if f.Status != nil {
		w.eq(`"status"`, *f.Status)
	}
	if f.StatusNot != nil {
		w.ne(`"status"`, *f.StatusNot)
	}
	if f.Trigger != nil {
		w.eq(`"trigger"`, *f.Trigger)
	}
	return w
}

// GetDeployment returns the deployment with this id.
func (s *Store) GetDeployment(ctx context.Context, id string) (*Deployment, error) {
	return queryOne(ctx, s.reader(), "get deployment", `SELECT `+deploymentCols+` FROM "deployments" WHERE "id" = ?`, []any{id}, scanDeployment)
}

// FindDeployment returns the first deployment matching f in the given order.
func (s *Store) FindDeployment(ctx context.Context, f DeploymentFilter, order Order) (*Deployment, error) {
	w := f.where()
	return queryOne(ctx, s.reader(), "find deployment",
		`SELECT `+deploymentCols+` FROM "deployments"`+w.clause()+order.clause()+` LIMIT 1`, w.args, scanDeployment)
}

// Page limits a list; Limit <= 0 means no limit.
type Page struct {
	Limit  int
	Offset int
}

func (p Page) clause() string {
	if p.Limit <= 0 && p.Offset <= 0 {
		return ""
	}
	limit := p.Limit
	if limit <= 0 {
		limit = -1
	}
	return fmt.Sprintf(" LIMIT %d OFFSET %d", limit, max(p.Offset, 0))
}

// ListDeployments returns deployments matching f, ordered and paginated.
func (s *Store) ListDeployments(ctx context.Context, f DeploymentFilter, order Order, page Page) ([]*Deployment, error) {
	w := f.where()
	return queryList(ctx, s.reader(), "list deployments",
		`SELECT `+deploymentCols+` FROM "deployments"`+w.clause()+order.clause()+page.clause(), w.args, scanDeployment)
}

// CountDeployments counts deployments matching f.
func (s *Store) CountDeployments(ctx context.Context, f DeploymentFilter) (int, error) {
	return count(ctx, s.reader(), "count deployments", `"deployments"`, f.where())
}

// ListDeploymentImageTags returns only image_tags of the newest `limit`
// successful deployments of a project (keep-2 image retention scan), skipping
// the potentially large logs column.
func (s *Store) ListDeploymentImageTags(ctx context.Context, projectID string, limit int) ([]*db.JSON, error) {
	return queryList(ctx, s.reader(), "list deployment image tags",
		`SELECT "image_tags" FROM "deployments" WHERE "project_id" = ? AND "status" = 'success' ORDER BY "created_at" DESC`+Page{Limit: limit}.clause(),
		[]any{projectID}, func(r rowScanner) (*db.JSON, error) {
			var j *db.JSON
			err := r.Scan(&j)
			return j, err
		})
}

// DeploymentCreate holds a new deployment's fields. Trigger defaults to "manual".
type DeploymentCreate struct {
	ProjectID     string
	Status        *string
	Trigger       *string
	CommitMessage *string
	CommitSHA     *string
	ImageTags     *db.JSON
	Logs          *string
	FinishedAt    *db.Time
}

// CreateDeployment inserts a deployment and returns the stored row.
func (s *Store) CreateDeployment(ctx context.Context, in DeploymentCreate) (*Deployment, error) {
	now := db.Now()
	d := &Deployment{ID: db.NewID(), ProjectID: in.ProjectID, Status: in.Status, Trigger: in.Trigger,
		CommitMessage: in.CommitMessage, CommitSHA: in.CommitSHA, ImageTags: in.ImageTags, Logs: in.Logs,
		CreatedAt: &now, FinishedAt: in.FinishedAt}
	if d.Trigger == nil {
		d.Trigger = Ptr("manual")
	}
	_, err := execAffected(ctx, s.writer(), "create deployment",
		`INSERT INTO "deployments" (`+deploymentCols+`) VALUES (`+placeholders(10)+`)`,
		d.ID, d.ProjectID, d.Status, d.Trigger, d.CommitMessage, d.CommitSHA, d.ImageTags, d.Logs, d.CreatedAt, d.FinishedAt)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// DeploymentUpdate lists the deployment columns to change.
type DeploymentUpdate struct {
	Status        Opt[*string]
	Trigger       Opt[*string]
	CommitMessage Opt[*string]
	CommitSHA     Opt[*string]
	ImageTags     Opt[*db.JSON]
	Logs          Opt[*string]
	FinishedAt    Opt[*db.Time]
}

func (in DeploymentUpdate) sets() sets {
	var set sets
	setOpt(&set, `"status"`, in.Status)
	setOpt(&set, `"trigger"`, in.Trigger)
	setOpt(&set, `"commit_message"`, in.CommitMessage)
	setOpt(&set, `"commit_sha"`, in.CommitSHA)
	setOpt(&set, `"image_tags"`, in.ImageTags)
	setOpt(&set, `"logs"`, in.Logs)
	setOpt(&set, `"finished_at"`, in.FinishedAt)
	return set
}

// UpdateDeployment applies the set fields and returns the updated row.
func (s *Store) UpdateDeployment(ctx context.Context, id string, in DeploymentUpdate) (*Deployment, error) {
	set := in.sets()
	if len(set.cols) == 0 {
		return s.GetDeployment(ctx, id)
	}
	return queryOne(ctx, s.writer(), "update deployment",
		`UPDATE "deployments" SET `+set.clause()+` WHERE "id" = ? RETURNING `+deploymentCols,
		append(set.args, id), scanDeployment)
}

// UpdateDeployments applies the set fields to every deployment matching f and
// returns how many rows changed (prisma.deployment.updateMany).
func (s *Store) UpdateDeployments(ctx context.Context, f DeploymentFilter, in DeploymentUpdate) (int64, error) {
	set := in.sets()
	if len(set.cols) == 0 {
		return 0, nil
	}
	w := f.where()
	return execAffected(ctx, s.writer(), "update deployments",
		`UPDATE "deployments" SET `+set.clause()+w.clause(), append(set.args, w.args...)...)
}
