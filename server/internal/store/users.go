package store

import (
	"context"

	"docklift/internal/db"
)

// User is a row of "users". Password (the bcrypt hash) is never serialised.
type User struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Email             string   `json:"email"`
	Password          string   `json:"-"`
	Role              string   `json:"role"`
	PasswordChangedAt *db.Time `json:"passwordChangedAt"`
	CreatedAt         db.Time  `json:"created_at"`
}

// PasswordVersion is the `pwdv` JWT claim for this user: passwordChangedAt in
// Unix milliseconds, or 0 when it was never set.
func (u *User) PasswordVersion() int64 {
	if u.PasswordChangedAt == nil {
		return 0
	}
	return u.PasswordChangedAt.UnixMilli()
}

const userCols = `"id", "name", "email", "password", "role", "passwordChangedAt", "created_at"`

func scanUser(r rowScanner) (*User, error) {
	var u User
	if err := r.Scan(&u.ID, &u.Name, &u.Email, &u.Password, &u.Role, &u.PasswordChangedAt, &u.CreatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

// CountUsers counts all users.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	return count(ctx, s.reader(), "count users", `"users"`, where{})
}

// CountUsersByRole counts users with the given role.
func (s *Store) CountUsersByRole(ctx context.Context, role string) (int, error) {
	var w where
	w.eq(`"role"`, role)
	return count(ctx, s.reader(), "count users by role", `"users"`, w)
}

// GetUser returns the user with this id.
func (s *Store) GetUser(ctx context.Context, id string) (*User, error) {
	return queryOne(ctx, s.reader(), "get user", `SELECT `+userCols+` FROM "users" WHERE "id" = ?`, []any{id}, scanUser)
}

// GetUserByEmail returns the user with this exact email (callers lowercase it).
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return queryOne(ctx, s.reader(), "get user by email", `SELECT `+userCols+` FROM "users" WHERE "email" = ?`, []any{email}, scanUser)
}

// FirstUserByRole returns the first user (natural order) with the given role,
// like prisma.user.findFirst({ where: { role } }).
func (s *Store) FirstUserByRole(ctx context.Context, role string) (*User, error) {
	return queryOne(ctx, s.reader(), "first user by role", `SELECT `+userCols+` FROM "users" WHERE "role" = ? LIMIT 1`, []any{role}, scanUser)
}

// OldestUser returns the earliest created user.
func (s *Store) OldestUser(ctx context.Context) (*User, error) {
	return queryOne(ctx, s.reader(), "oldest user", `SELECT `+userCols+` FROM "users" ORDER BY "created_at" ASC LIMIT 1`, nil, scanUser)
}

// UserCreate holds the fields of a new user. Role defaults to "admin".
type UserCreate struct {
	Name              string
	Email             string
	Password          string
	Role              string
	PasswordChangedAt *db.Time
}

// CreateUser inserts a user; a duplicate email is ErrConflict.
func (s *Store) CreateUser(ctx context.Context, in UserCreate) (*User, error) {
	u := &User{
		ID:                db.NewID(),
		Name:              in.Name,
		Email:             in.Email,
		Password:          in.Password,
		Role:              in.Role,
		PasswordChangedAt: in.PasswordChangedAt,
		CreatedAt:         db.Now(),
	}
	if u.Role == "" {
		u.Role = "admin"
	}
	_, err := execAffected(ctx, s.writer(), "create user",
		`INSERT INTO "users" (`+userCols+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Name, u.Email, u.Password, u.Role, u.PasswordChangedAt, u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// UserUpdate lists the user columns to change.
type UserUpdate struct {
	Name              Opt[string]
	Email             Opt[string]
	Password          Opt[string]
	Role              Opt[string]
	PasswordChangedAt Opt[*db.Time]
}

// UpdateUser applies the set fields and returns the updated row. A missing id
// is ErrNotFound; a duplicate email is ErrConflict.
func (s *Store) UpdateUser(ctx context.Context, id string, in UserUpdate) (*User, error) {
	var set sets
	setOpt(&set, `"name"`, in.Name)
	setOpt(&set, `"email"`, in.Email)
	setOpt(&set, `"password"`, in.Password)
	setOpt(&set, `"role"`, in.Role)
	setOpt(&set, `"passwordChangedAt"`, in.PasswordChangedAt)
	if len(set.cols) == 0 {
		return s.GetUser(ctx, id)
	}
	return queryOne(ctx, s.writer(), "update user",
		`UPDATE "users" SET `+set.clause()+` WHERE "id" = ? RETURNING `+userCols,
		append(set.args, id), scanUser)
}
