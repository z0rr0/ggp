package databaser

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jmoiron/sqlx"
)

// ErrUserNotFound is returned when a user operation fails because the user doesn't exist.
var ErrUserNotFound = errors.New("user not found")

// User status constants.
const (
	userPending  = 0
	userApproved = 1
	userRejected = 2
)

// User represents a user in the database.
type User struct {
	Created   time.Time `db:"created"`
	Updated   time.Time `db:"updated"`
	Username  string    `db:"username"`
	FirstName string    `db:"first_name"`
	LastName  string    `db:"last_name"`
	ID        int64     `db:"id"`
	Status    uint8     `db:"status"`
}

// String implements stringer for User.
func (user *User) String() string {
	return fmt.Sprintf("{ID: %d, Status: %d, Username: '%s', FirstName: '%s', LastName: '%s', Created: '%s', Updated: '%s'}",
		user.ID, user.Status, user.Username, user.FirstName, user.LastName,
		user.Created.Format(time.RFC3339), user.Updated.Format(time.RFC3339))
}

// IsPending checks if the user is pending.
func (user *User) IsPending() bool {
	return user.Status == userPending
}

// IsApproved checks if the user is approved.
func (user *User) IsApproved() bool {
	return user.Status == userApproved
}

// IsRejected checks if the user is rejected.
func (user *User) IsRejected() bool {
	return user.Status == userRejected
}

// LogValue implements slog.LogValuer for User.
func (user *User) LogValue() slog.Value {
	return slog.StringValue(user.String())
}

// GetUser retrieves a user by ID from the database.
func (db *DB) GetUser(ctx context.Context, userID int64) (*User, error) {
	const query = `SELECT id, status, username, first_name, last_name, created, updated FROM users WHERE id = ?;`

	var user User
	err := db.GetContext(ctx, &user, query, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: id %d", ErrUserNotFound, userID)
		}
		return nil, fmt.Errorf("select user: %w", err)
	}

	return &user, nil
}

// GetUsers retrieves all users from the database.
func (db *DB) GetUsers(ctx context.Context) ([]User, error) {
	const query = `SELECT id, status, username, first_name, last_name, created, updated 
		FROM users ORDER BY status, updated, id;`

	var users []User
	err := db.SelectContext(ctx, &users, query)
	if err != nil {
		return nil, fmt.Errorf("select users: %w", err)
	}

	return users, nil
}

// GetApprovedUsers retrieves all approved users from the database.
func (db *DB) GetApprovedUsers(ctx context.Context) ([]User, error) {
	const query = `SELECT id, status, username, first_name, last_name, created, updated FROM users WHERE status = ?;`

	var users []User
	err := db.SelectContext(ctx, &users, query, userApproved)
	if err != nil {
		return nil, fmt.Errorf("select approved users: %w", err)
	}

	return users, nil
}

// GetPendingUsers retrieves all pending users from the database.
func (db *DB) GetPendingUsers(ctx context.Context) ([]User, error) {
	const query = `SELECT id, status, username, first_name, last_name, created, updated FROM users WHERE status = ?;`

	var users []User
	err := db.SelectContext(ctx, &users, query, userPending)
	if err != nil {
		return nil, fmt.Errorf("select pending users: %w", err)
	}

	return users, nil
}

// ApproveUser sets the approved flag to true for a user by ID.
func (db *DB) ApproveUser(ctx context.Context, userID int64) error {
	const query = `UPDATE users SET status = ?, updated = ? WHERE id = ? AND status = ?;`

	result, err := db.ExecContext(ctx, query, userApproved, time.Now().UTC(), userID, userPending)
	if err != nil {
		return fmt.Errorf("update user approval: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected for user approval: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("approve user: %w: id %d", ErrUserNotFound, userID)
	}

	return nil
}

// RejectUser sets the approved flag to false for a user by ID.
func (db *DB) RejectUser(ctx context.Context, userID int64) error {
	const query = `UPDATE users SET status = ?, updated = ? WHERE id = ? AND status != ?;`

	result, err := db.ExecContext(ctx, query, userRejected, time.Now().UTC(), userID, userRejected)
	if err != nil {
		return fmt.Errorf("update user rejection: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected for user rejection: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("reject user: %w: id %d", ErrUserNotFound, userID)
	}

	return nil
}

// DeleteUser removes a user by ID from the database.
func (db *DB) DeleteUser(ctx context.Context, userID int64) error {
	const query = `DELETE FROM users WHERE id = ?;`

	result, err := db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected for delete user: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("delete user: %w: id %d", ErrUserNotFound, userID)
	}

	return nil
}

// GetOrCreateUser retrieves a user by ID or creates a new one if not found.
func GetOrCreateUser(ctx context.Context, tx *sqlx.Tx, id int64, username, firstName, lastName string) (*User, error) {
	const (
		queryInsert = `INSERT INTO users (id, status, username, first_name, last_name, created, updated) 
			VALUES (:id, 0, :username, :first_name, :last_name, :created, :updated);`
		querySelect = `SELECT id, status, username, first_name, last_name, created, updated FROM users WHERE id = ?;`
	)

	// try to find an existing user
	var user User

	err := tx.GetContext(ctx, &user, querySelect, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			slog.DebugContext(ctx, "user not found, creating new", "id", id)
		} else {
			return nil, fmt.Errorf("select user: %w", err)
		}
	} else {
		return &user, nil
	}

	// create a new user
	now := time.Now().UTC()
	user = User{
		ID:        id,
		Status:    userPending,
		Username:  username,
		FirstName: firstName,
		LastName:  lastName,
		Created:   now,
		Updated:   now,
	}

	result, err := tx.NamedExecContext(ctx, queryInsert, &user)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("get rows affected for insert user: %w", err)
	}

	if rowsAffected == 0 {
		return nil, fmt.Errorf("no user affected with id %d to create", id)
	}

	slog.InfoContext(ctx, "created new user", "user", &user)
	return &user, nil
}
