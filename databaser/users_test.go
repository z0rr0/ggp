package databaser

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

func TestUser_String(t *testing.T) {
	now := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	user := User{
		ID:        123,
		Status:    userApproved,
		Username:  "testuser",
		FirstName: "John",
		LastName:  "Doe",
		Created:   now,
		Updated:   now,
	}

	got := user.String()

	// Verify key fields are present
	if !contains(got, "123") {
		t.Errorf("String() missing ID, got: %s", got)
	}
	if !contains(got, "testuser") {
		t.Errorf("String() missing username, got: %s", got)
	}
	if !contains(got, "John") {
		t.Errorf("String() missing first_name, got: %s", got)
	}
}

func TestUser_StatusMethods(t *testing.T) {
	tests := []struct {
		name         string
		status       uint8
		wantPending  bool
		wantApproved bool
		wantRejected bool
	}{
		{
			name:         "pending user",
			status:       userPending,
			wantPending:  true,
			wantApproved: false,
			wantRejected: false,
		},
		{
			name:         "approved user",
			status:       userApproved,
			wantPending:  false,
			wantApproved: true,
			wantRejected: false,
		},
		{
			name:         "rejected user",
			status:       userRejected,
			wantPending:  false,
			wantApproved: false,
			wantRejected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{Status: tt.status}
			if got := user.IsPending(); got != tt.wantPending {
				t.Errorf("IsPending() = %v, want %v", got, tt.wantPending)
			}
			if got := user.IsApproved(); got != tt.wantApproved {
				t.Errorf("IsApproved() = %v, want %v", got, tt.wantApproved)
			}
			if got := user.IsRejected(); got != tt.wantRejected {
				t.Errorf("IsRejected() = %v, want %v", got, tt.wantRejected)
			}
		})
	}
}

func TestUser_LogValue(t *testing.T) {
	user := &User{
		ID:        456,
		Username:  "logtest",
		FirstName: "Jane",
		LastName:  "Smith",
		Created:   time.Now().UTC(),
		Updated:   time.Now().UTC(),
	}

	got := user.LogValue().String()
	if got == "" {
		t.Error("LogValue() returned empty string")
	}
	if !contains(got, "456") || !contains(got, "logtest") {
		t.Errorf("LogValue() = %q, should contain ID and username", got)
	}
}

func TestGetUser(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Insert a test user
	now := time.Now().UTC().Truncate(time.Second)
	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, status, username, first_name, last_name, created, updated) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		100, userApproved, "testuser", "Test", "User", now, now)
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}

	tests := []struct {
		name       string
		userID     int64
		wantErr    bool
		wantNotErr error
	}{
		{
			name:    "existing user",
			userID:  100,
			wantErr: false,
		},
		{
			name:       "non-existent user",
			userID:     999,
			wantErr:    true,
			wantNotErr: ErrUserNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, userErr := db.GetUser(ctx, tt.userID)
			if (userErr != nil) != tt.wantErr {
				t.Errorf("GetUser() error = %v, wantErr %v", userErr, tt.wantErr)
				return
			}
			if tt.wantNotErr != nil && !errors.Is(userErr, tt.wantNotErr) {
				t.Errorf("GetUser() error = %v, want %v", userErr, tt.wantNotErr)
			}
			if userErr == nil {
				if user.ID != tt.userID {
					t.Errorf("GetUser() ID = %d, want %d", user.ID, tt.userID)
				}
				if user.Username != "testuser" {
					t.Errorf("GetUser() Username = %q, want %q", user.Username, "testuser")
				}
			}
		})
	}
}

func TestGetUsers(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Empty database
	users, err := db.GetUsers(ctx)
	if err != nil {
		t.Fatalf("GetUsers() on empty db error = %v", err)
	}
	if len(users) != 0 {
		t.Errorf("GetUsers() on empty db returned %d users, want 0", len(users))
	}

	// Insert test users with different statuses
	now := time.Now().UTC().Truncate(time.Second)
	testUsers := []struct {
		id       int64
		status   uint8
		username string
	}{
		{1, userPending, "pending1"},
		{2, userApproved, "approved1"},
		{3, userRejected, "rejected1"},
		{4, userPending, "pending2"},
	}

	for _, u := range testUsers {
		_, err = db.ExecContext(ctx,
			`INSERT INTO users (id, status, username, first_name, last_name, created, updated) VALUES (?, ?, ?, '', '', ?, ?)`,
			u.id, u.status, u.username, now, now)
		if err != nil {
			t.Fatalf("failed to insert user %d: %v", u.id, err)
		}
	}

	users, err = db.GetUsers(ctx)
	if err != nil {
		t.Fatalf("GetUsers() error = %v", err)
	}
	if len(users) != 4 {
		t.Errorf("GetUsers() returned %d users, want 4", len(users))
	}

	// Verify ordering by status (pending=0 first, then approved=1, then rejected=2)
	if len(users) >= 2 && users[0].Status > users[len(users)-1].Status {
		t.Error("GetUsers() should order by status ascending")
	}
}

func TestGetApprovedUsers(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Empty database
	users, err := db.GetApprovedUsers(ctx)
	if err != nil {
		t.Fatalf("GetApprovedUsers() on empty db error = %v", err)
	}
	if len(users) != 0 {
		t.Errorf("GetApprovedUsers() on empty db returned %d, want 0", len(users))
	}

	// Insert mixed users
	now := time.Now().UTC().Truncate(time.Second)
	_, err = db.ExecContext(ctx,
		`INSERT INTO users (id, status, username, first_name, last_name, created, updated) VALUES
		(1, ?, 'pending', '', '', ?, ?),
		(2, ?, 'approved1', '', '', ?, ?),
		(3, ?, 'approved2', '', '', ?, ?),
		(4, ?, 'rejected', '', '', ?, ?)`,
		userPending, now, now,
		userApproved, now, now,
		userApproved, now, now,
		userRejected, now, now)
	if err != nil {
		t.Fatalf("failed to insert users: %v", err)
	}

	users, err = db.GetApprovedUsers(ctx)
	if err != nil {
		t.Fatalf("GetApprovedUsers() error = %v", err)
	}
	if len(users) != 2 {
		t.Errorf("GetApprovedUsers() returned %d, want 2", len(users))
	}

	for _, u := range users {
		if !u.IsApproved() {
			t.Errorf("GetApprovedUsers() returned non-approved user: %v", u)
		}
	}
}

func TestGetPendingUsers(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Empty database
	users, err := db.GetPendingUsers(ctx)
	if err != nil {
		t.Fatalf("GetPendingUsers() on empty db error = %v", err)
	}
	if len(users) != 0 {
		t.Errorf("GetPendingUsers() on empty db returned %d, want 0", len(users))
	}

	// Insert mixed users
	now := time.Now().UTC().Truncate(time.Second)
	_, err = db.ExecContext(ctx,
		`INSERT INTO users (id, status, username, first_name, last_name, created, updated) VALUES
		(1, ?, 'pending1', '', '', ?, ?),
		(2, ?, 'pending2', '', '', ?, ?),
		(3, ?, 'approved', '', '', ?, ?),
		(4, ?, 'rejected', '', '', ?, ?)`,
		userPending, now, now,
		userPending, now, now,
		userApproved, now, now,
		userRejected, now, now)
	if err != nil {
		t.Fatalf("failed to insert users: %v", err)
	}

	users, err = db.GetPendingUsers(ctx)
	if err != nil {
		t.Fatalf("GetPendingUsers() error = %v", err)
	}
	if len(users) != 2 {
		t.Errorf("GetPendingUsers() returned %d, want 2", len(users))
	}

	for _, u := range users {
		if !u.IsPending() {
			t.Errorf("GetPendingUsers() returned non-pending user: %v", u)
		}
	}
}

func TestApproveUser(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)

	tests := []struct {
		name       string
		setup      func() int64
		wantErr    bool
		wantNotErr error
	}{
		{
			name: "approve pending user",
			setup: func() int64 {
				_, err := db.ExecContext(ctx,
					`INSERT INTO users (id, status, username, first_name, last_name, created, updated) VALUES (?, ?, '', '', '', ?, ?)`,
					1, userPending, now, now)
				if err != nil {
					t.Fatalf("setup failed: %v", err)
				}
				return 1
			},
			wantErr: false,
		},
		{
			name: "approve non-existent user",
			setup: func() int64 {
				return 999
			},
			wantErr:    true,
			wantNotErr: ErrUserNotFound,
		},
		{
			name: "approve already approved user",
			setup: func() int64 {
				_, err := db.ExecContext(ctx,
					`INSERT INTO users (id, status, username, first_name, last_name, created, updated) VALUES (?, ?, '', '', '', ?, ?)`,
					2, userApproved, now, now)
				if err != nil {
					t.Fatalf("setup failed: %v", err)
				}
				return 2
			},
			wantErr:    true,
			wantNotErr: ErrUserNotFound,
		},
		{
			name: "approve rejected user",
			setup: func() int64 {
				_, err := db.ExecContext(ctx,
					`INSERT INTO users (id, status, username, first_name, last_name, created, updated) VALUES (?, ?, '', '', '', ?, ?)`,
					3, userRejected, now, now)
				if err != nil {
					t.Fatalf("setup failed: %v", err)
				}
				return 3
			},
			wantErr:    true,
			wantNotErr: ErrUserNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := tt.setup()
			err := db.ApproveUser(ctx, userID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ApproveUser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantNotErr != nil && !errors.Is(err, tt.wantNotErr) {
				t.Errorf("ApproveUser() error = %v, want %v", err, tt.wantNotErr)
			}
			if err == nil {
				user, err := db.GetUser(ctx, userID)
				if err != nil {
					t.Fatalf("GetUser() after approve error = %v", err)
				}
				if !user.IsApproved() {
					t.Errorf("user status = %d, want approved", user.Status)
				}
			}
		})
	}
}

func TestRejectUser(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)

	tests := []struct {
		name       string
		setup      func() int64
		wantErr    bool
		wantNotErr error
	}{
		{
			name: "reject pending user",
			setup: func() int64 {
				_, err := db.ExecContext(ctx,
					`INSERT INTO users (id, status, username, first_name, last_name, created, updated) VALUES (?, ?, '', '', '', ?, ?)`,
					10, userPending, now, now)
				if err != nil {
					t.Fatalf("setup failed: %v", err)
				}
				return 10
			},
			wantErr: false,
		},
		{
			name: "reject approved user",
			setup: func() int64 {
				_, err := db.ExecContext(ctx,
					`INSERT INTO users (id, status, username, first_name, last_name, created, updated) VALUES (?, ?, '', '', '', ?, ?)`,
					11, userApproved, now, now)
				if err != nil {
					t.Fatalf("setup failed: %v", err)
				}
				return 11
			},
			wantErr: false,
		},
		{
			name: "reject non-existent user",
			setup: func() int64 {
				return 999
			},
			wantErr:    true,
			wantNotErr: ErrUserNotFound,
		},
		{
			name: "reject already rejected user",
			setup: func() int64 {
				_, err := db.ExecContext(ctx,
					`INSERT INTO users (id, status, username, first_name, last_name, created, updated) VALUES (?, ?, '', '', '', ?, ?)`,
					12, userRejected, now, now)
				if err != nil {
					t.Fatalf("setup failed: %v", err)
				}
				return 12
			},
			wantErr:    true,
			wantNotErr: ErrUserNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := tt.setup()
			err := db.RejectUser(ctx, userID)
			if (err != nil) != tt.wantErr {
				t.Errorf("RejectUser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantNotErr != nil && !errors.Is(err, tt.wantNotErr) {
				t.Errorf("RejectUser() error = %v, want %v", err, tt.wantNotErr)
			}
			if err == nil {
				user, err := db.GetUser(ctx, userID)
				if err != nil {
					t.Fatalf("GetUser() after reject error = %v", err)
				}
				if !user.IsRejected() {
					t.Errorf("user status = %d, want rejected", user.Status)
				}
			}
		})
	}
}

func TestDeleteUser(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)

	tests := []struct {
		name       string
		setup      func() int64
		wantErr    bool
		wantNotErr error
	}{
		{
			name: "delete existing user",
			setup: func() int64 {
				_, err := db.ExecContext(ctx,
					`INSERT INTO users (id, status, username, first_name, last_name, created, updated) VALUES (?, ?, '', '', '', ?, ?)`,
					20, userPending, now, now)
				if err != nil {
					t.Fatalf("setup failed: %v", err)
				}
				return 20
			},
			wantErr: false,
		},
		{
			name: "delete non-existent user",
			setup: func() int64 {
				return 999
			},
			wantErr:    true,
			wantNotErr: ErrUserNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := tt.setup()
			err := db.DeleteUser(ctx, userID)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteUser() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantNotErr != nil && !errors.Is(err, tt.wantNotErr) {
				t.Errorf("DeleteUser() error = %v, want %v", err, tt.wantNotErr)
			}
			if err == nil {
				_, err = db.GetUser(ctx, userID)
				if !errors.Is(err, ErrUserNotFound) {
					t.Errorf("GetUser() after delete should return ErrUserNotFound, got %v", err)
				}
			}
		})
	}
}

func TestDeleteUser_DeleteTwice(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, status, username, first_name, last_name, created, updated) VALUES (?, ?, '', '', '', ?, ?)`,
		30, userPending, now, now)
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	if err = db.DeleteUser(ctx, 30); err != nil {
		t.Fatalf("first DeleteUser() error = %v", err)
	}

	err = db.DeleteUser(ctx, 30)
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("second DeleteUser() error = %v, want ErrUserNotFound", err)
	}
}

func TestGetOrCreateUser_CreateNew(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	var user *User
	err := InTransaction(ctx, db, func(tx *sqlx.Tx) error {
		var err error
		user, err = GetOrCreateUser(ctx, tx, 100, "newuser", "New", "User")
		return err
	})
	if err != nil {
		t.Fatalf("GetOrCreateUser() error = %v", err)
	}

	if user.ID != 100 {
		t.Errorf("user.ID = %d, want 100", user.ID)
	}
	if user.Username != "newuser" {
		t.Errorf("user.Username = %q, want %q", user.Username, "newuser")
	}
	if user.FirstName != "New" {
		t.Errorf("user.FirstName = %q, want %q", user.FirstName, "New")
	}
	if user.LastName != "User" {
		t.Errorf("user.LastName = %q, want %q", user.LastName, "User")
	}
	if !user.IsPending() {
		t.Errorf("new user should be pending, got status %d", user.Status)
	}
	if user.Created.IsZero() {
		t.Error("user.Created should not be zero")
	}
}

func TestGetOrCreateUser_GetExisting(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Insert existing user
	now := time.Now().UTC().Truncate(time.Second)
	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, status, username, first_name, last_name, created, updated) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		200, userApproved, "existinguser", "Existing", "User", now, now)
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	var user *User
	err = InTransaction(ctx, db, func(tx *sqlx.Tx) error {
		var txErr error
		// Call with different data - should return existing user
		user, txErr = GetOrCreateUser(ctx, tx, 200, "different", "Different", "Name")
		return txErr
	})
	if err != nil {
		t.Fatalf("GetOrCreateUser() error = %v", err)
	}

	// Should return existing user data, not the new parameters
	if user.Username != "existinguser" {
		t.Errorf("user.Username = %q, want %q", user.Username, "existinguser")
	}
	if user.FirstName != "Existing" {
		t.Errorf("user.FirstName = %q, want %q", user.FirstName, "Existing")
	}
	if !user.IsApproved() {
		t.Errorf("existing user should remain approved, got status %d", user.Status)
	}
}

func TestGetOrCreateUser_EmptyFields(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	var user *User
	err := InTransaction(ctx, db, func(tx *sqlx.Tx) error {
		var err error
		user, err = GetOrCreateUser(ctx, tx, 300, "", "", "")
		return err
	})
	if err != nil {
		t.Fatalf("GetOrCreateUser() with empty fields error = %v", err)
	}

	if user.ID != 300 {
		t.Errorf("user.ID = %d, want 300", user.ID)
	}
	if user.Username != "" {
		t.Errorf("user.Username = %q, want empty", user.Username)
	}
}

func TestGetOrCreateUser_TransactionRollback(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	testErr := errors.New("forced error")
	err := InTransaction(ctx, db, func(tx *sqlx.Tx) error {
		_, err := GetOrCreateUser(ctx, tx, 400, "rollbackuser", "Rollback", "User")
		if err != nil {
			return err
		}
		return testErr
	})

	if err == nil {
		t.Fatal("expected error from transaction")
	}

	// User should not exist due to rollback
	_, err = db.GetUser(ctx, 400)
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("GetUser() after rollback should return ErrUserNotFound, got %v", err)
	}
}

func TestApproveUser_UpdatesTimestamp(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Insert user with old timestamp
	oldTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, status, username, first_name, last_name, created, updated) VALUES (?, ?, '', '', '', ?, ?)`,
		500, userPending, oldTime, oldTime)
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	beforeApprove := time.Now().UTC()
	if err := db.ApproveUser(ctx, 500); err != nil {
		t.Fatalf("ApproveUser() error = %v", err)
	}

	user, err := db.GetUser(ctx, 500)
	if err != nil {
		t.Fatalf("GetUser() error = %v", err)
	}

	if user.Updated.Before(beforeApprove) {
		t.Errorf("user.Updated = %v, should be after %v", user.Updated, beforeApprove)
	}
	if !user.Created.Equal(oldTime) {
		t.Errorf("user.Created = %v, should remain %v", user.Created, oldTime)
	}
}

func TestRejectUser_UpdatesTimestamp(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	oldTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, status, username, first_name, last_name, created, updated) VALUES (?, ?, '', '', '', ?, ?)`,
		600, userPending, oldTime, oldTime)
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	beforeReject := time.Now().UTC()
	if err = db.RejectUser(ctx, 600); err != nil {
		t.Fatalf("RejectUser() error = %v", err)
	}

	user, err := db.GetUser(ctx, 600)
	if err != nil {
		t.Fatalf("GetUser() error = %v", err)
	}

	if user.Updated.Before(beforeReject) {
		t.Errorf("user.Updated = %v, should be after %v", user.Updated, beforeReject)
	}
}

func TestErrUserNotFound_ErrorsIs(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	_, err := db.GetUser(ctx, 99999)
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}

	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("errors.Is(err, ErrUserNotFound) = false, want true; err = %v", err)
	}
}
