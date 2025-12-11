# Code Review: watcher package (middlewares.go, admin.go, watcher.go)

## Issues Found

### middlewares.go

#### 1. Nil pointer dereference risk (lines 47, 65)
**Severity**: High
**Issue**: `BotAdminOnlyMiddleware` and `BotAuthMiddleware` access `update.Message.From.ID` without checking if `update.Message` or `update.Message.From` is nil.
**Fix**: Add nil checks before accessing nested fields.

#### 2. Parameter naming convention (lines 44, 62)
**Severity**: Low (linter flagged)
**Issue**: `adminUserIds` should be `adminUserIDs` per Go naming conventions.
**Fix**: Rename parameter.

#### 3. Inefficient user lookup (lines 72-84)
**Severity**: Medium
**Issue**: `BotAuthMiddleware` calls `GetApprovedUsers()` and iterates through all users to find a match. This is O(n) and inefficient.
**Fix**: Use `GetUser()` directly and check if approved, or create a new `IsUserApproved()` method.

### admin.go

#### 1. Code duplication (lines 88-126 vs 129-167)
**Severity**: Low (linter flagged as dupl)
**Issue**: `HandleApprove` and `HandleReject` have nearly identical structure.
**Fix**: Could extract common parsing logic, but may reduce readability. Keep as-is for now.

#### 2. Wrong comment (line 158)
**Severity**: Low
**Issue**: Comment says "notify users about approval" but code handles rejection.
**Fix**: Update comment to "notify users about rejection".

#### 3. Missing nil checks
**Severity**: High
**Issue**: Handlers access `update.Message.Text`, `update.Message.Chat.ID` without nil checks.
**Fix**: Add guard clauses at the beginning of each handler.

### watcher.go

#### 1. Nil pointer dereference in HandleStart (line 122-123)
**Severity**: High
**Issue**: Accesses `update.Message.From.ID` without checking for nil. Test "nil message" case panics.
**Fix**: Add nil check at the beginning.

#### 2. `isAuthorized` only checks admins (lines 282-284)
**Severity**: Medium
**Issue**: The `isAuthorized` method only checks if user is in `adminIDs`, not if they're an approved user from DB. This means `handlePeriod` and `DefaultHandler` only work for admins.
**Fix**: Either rename to `isAdmin` or extend to check DB approved users.

## Changes Made

1. Added nil checks in all middleware and handler functions
2. Renamed `adminUserIds` to `adminUserIDs`
3. Fixed comment in HandleReject
4. Optimized BotAuthMiddleware to use direct user lookup
5. Added nil checks in HandleStart, HandleStop, HandleID
6. Added nil checks in admin handlers
