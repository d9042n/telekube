package middleware

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/config"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// ─── In-memory storage stub ───────────────────────────────────────────────────

type memUserRepo struct {
	mu    sync.RWMutex
	users map[int64]*entity.User
	err   error // if set, all calls return this error
}

func newMemUserRepo() *memUserRepo {
	return &memUserRepo{users: make(map[int64]*entity.User)}
}

func (r *memUserRepo) GetByTelegramID(_ context.Context, id int64) (*entity.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.err != nil {
		return nil, r.err
	}
	u, ok := r.users[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return u, nil
}

func (r *memUserRepo) Upsert(_ context.Context, u *entity.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.users[u.TelegramID] = u
	return nil
}

func (r *memUserRepo) List(_ context.Context) ([]entity.User, error) { return nil, nil }

// memStorage wraps any UserRepository to satisfy storage.Storage.
type memStorage struct {
	users storage.UserRepository
}

func (s *memStorage) Users() storage.UserRepository    { return s.users }
func (s *memStorage) Audit() storage.AuditRepository   { return nil }
func (s *memStorage) RBAC() storage.RBACRepository     { return nil }
func (s *memStorage) Freeze() storage.FreezeRepository { return nil }
func (s *memStorage) Approval() storage.ApprovalRepository { return nil }
func (s *memStorage) NotificationPrefs() storage.NotificationPrefRepository { return nil }
func (s *memStorage) Close() error                          { return nil }
func (s *memStorage) Ping(_ context.Context) error          { return nil }

// ─── Fake telebot.Context ─────────────────────────────────────────────────────

// fakeTelebotContext captures what the middleware sends/sets.
type fakeTelebotContext struct {
	telebot.Context // embed to satisfy interface; override only what we need

	sender   *telebot.User
	chat     *telebot.Chat
	sent     []string
	ctxStore map[string]interface{}
	mu       sync.Mutex
}

func newFakeCtx(sender *telebot.User, chatID int64) *fakeTelebotContext {
	chat := &telebot.Chat{ID: chatID}
	if sender != nil && chatID == sender.ID {
		chat.ID = sender.ID
	}
	return &fakeTelebotContext{
		sender:   sender,
		chat:     chat,
		ctxStore: make(map[string]interface{}),
	}
}

func (f *fakeTelebotContext) Sender() *telebot.User { return f.sender }
func (f *fakeTelebotContext) Chat() *telebot.Chat   { return f.chat }

func (f *fakeTelebotContext) Send(what interface{}, _ ...interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := what.(string); ok {
		f.sent = append(f.sent, s)
	}
	return nil
}

func (f *fakeTelebotContext) Get(key string) interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ctxStore[key]
}

func (f *fakeTelebotContext) Set(key string, val interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ctxStore[key] = val
}



// ─── helpers ─────────────────────────────────────────────────────────────────

func defaultTelegramCfg() config.TelegramConfig {
	return config.TelegramConfig{
		Token:    "test-token",
		AdminIDs: []int64{1000},
	}
}

func buildAuth(store storage.Storage, cfg config.TelegramConfig) telebot.MiddlewareFunc {
	return Auth(store, cfg, zap.NewNop())
}

func runMiddleware(ctx *fakeTelebotContext, store storage.Storage, cfg config.TelegramConfig) (bool, []string) {
	called := false
	handler := func(c telebot.Context) error {
		called = true
		return nil
	}
	mw := buildAuth(store, cfg)
	_ = mw(handler)(ctx)
	return called, ctx.sent
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestAuth_NilSender_HandlerNotCalled(t *testing.T) {
	t.Parallel()

	store := &memStorage{users: newMemUserRepo()}
	ctx := newFakeCtx(nil, 42)

	called, sent := runMiddleware(ctx, store, defaultTelegramCfg())

	assert.False(t, called, "handler must not be called when sender is nil")
	assert.Empty(t, sent, "nothing should be sent when sender is nil")
}

func TestAuth_NewUser_NotInAdminIDs_RegisteredAsViewer(t *testing.T) {
	t.Parallel()

	store := &memStorage{users: newMemUserRepo()}
	sender := &telebot.User{ID: 42, Username: "alice", FirstName: "Alice"}
	ctx := newFakeCtx(sender, sender.ID)

	called, _ := runMiddleware(ctx, store, defaultTelegramCfg())
	require.True(t, called, "handler should be called for new user")

	user, err := store.Users().GetByTelegramID(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, entity.RoleViewer, user.Role)
	assert.Equal(t, "alice", user.Username)
	assert.True(t, user.IsActive)
}

func TestAuth_NewUser_InAdminIDs_RegisteredAsAdmin(t *testing.T) {
	t.Parallel()

	store := &memStorage{users: newMemUserRepo()}
	cfg := config.TelegramConfig{
		Token:    "test-token",
		AdminIDs: []int64{99},
	}
	sender := &telebot.User{ID: 99, Username: "root", FirstName: "Root"}
	ctx := newFakeCtx(sender, sender.ID)

	called, _ := runMiddleware(ctx, store, cfg)
	require.True(t, called)

	user, err := store.Users().GetByTelegramID(context.Background(), 99)
	require.NoError(t, err)
	assert.Equal(t, entity.RoleAdmin, user.Role, "user in AdminIDs must be registered as admin")
}

func TestAuth_AllowedChats_NotInList_Ignored(t *testing.T) {
	t.Parallel()

	store := &memStorage{users: newMemUserRepo()}
	cfg := config.TelegramConfig{
		Token:        "test-token",
		AdminIDs:     []int64{1000},
		AllowedChats: []int64{111, 222}, // only these chats allowed
	}
	sender := &telebot.User{ID: 42, Username: "alice", FirstName: "Alice"}
	// chatID is 333 — not in AllowedChats, not a private DM to sender.ID
	ctx := newFakeCtx(sender, 333)

	called, sent := runMiddleware(ctx, store, cfg)
	assert.False(t, called, "handler must not be called for disallowed chat")
	assert.Empty(t, sent, "no message must be sent for disallowed chat")
}

func TestAuth_AllowedChats_PrivateDM_AlwaysAllowed(t *testing.T) {
	t.Parallel()

	store := &memStorage{users: newMemUserRepo()}
	cfg := config.TelegramConfig{
		Token:        "test-token",
		AdminIDs:     []int64{1000},
		AllowedChats: []int64{111}, // group chat 111 allowed
	}
	sender := &telebot.User{ID: 42, Username: "alice", FirstName: "Alice"}
	// When chatID == senderID, it's a private chat — must still work.
	ctx := newFakeCtx(sender, sender.ID)

	called, _ := runMiddleware(ctx, store, cfg)
	assert.True(t, called, "private DM must always be allowed regardless of AllowedChats")
}

func TestAuth_AllowedChats_Empty_AllChatsAllowed(t *testing.T) {
	t.Parallel()

	store := &memStorage{users: newMemUserRepo()}
	cfg := config.TelegramConfig{
		Token:    "test-token",
		AdminIDs: []int64{1000},
		// AllowedChats empty → no restriction
	}
	sender := &telebot.User{ID: 42, Username: "alice", FirstName: "Alice"}
	ctx := newFakeCtx(sender, 9999) // any chat

	called, _ := runMiddleware(ctx, store, cfg)
	assert.True(t, called, "all chats must be allowed when AllowedChats is empty")
}

func TestAuth_StorageError_OnGetUser_SendsInternalError(t *testing.T) {
	t.Parallel()

	repo := newMemUserRepo()
	repo.err = errors.New("db connection refused")
	store := &memStorage{users: repo}

	sender := &telebot.User{ID: 42, Username: "alice", FirstName: "Alice"}
	ctx := newFakeCtx(sender, sender.ID)

	called, sent := runMiddleware(ctx, store, defaultTelegramCfg())
	assert.False(t, called, "handler must not be called on storage error")
	require.NotEmpty(t, sent)
	assert.Contains(t, sent[0], "⚠️", "error message must contain warning emoji")
}

func TestAuth_StorageError_OnUpsert_SendsInternalError(t *testing.T) {
	t.Parallel()

	// Use a special repo where GetByTelegramID returns ErrNotFound but Upsert fails.
	repo := &upsertErrUserRepo{users: make(map[int64]*entity.User)}
	store := &memStorage{users: repo}

	sender := &telebot.User{ID: 42, Username: "alice", FirstName: "Alice"}
	ctx := newFakeCtx(sender, sender.ID)

	called, sent := runMiddleware(ctx, store, defaultTelegramCfg())
	assert.False(t, called, "handler must not be called when upsert fails")
	require.NotEmpty(t, sent)
	assert.Contains(t, sent[0], "⚠️")
}

// upsertErrUserRepo: GetByTelegramID returns ErrNotFound, Upsert always errors.
type upsertErrUserRepo struct {
	users map[int64]*entity.User
}

func (r *upsertErrUserRepo) GetByTelegramID(_ context.Context, _ int64) (*entity.User, error) {
	return nil, storage.ErrNotFound
}
func (r *upsertErrUserRepo) Upsert(_ context.Context, _ *entity.User) error {
	return errors.New("upsert failed: disk full")
}
func (r *upsertErrUserRepo) List(_ context.Context) ([]entity.User, error) { return nil, nil }

func TestAuth_UserChangesUsername_DBUpdated(t *testing.T) {
	t.Parallel()

	repo := newMemUserRepo()
	existing := &entity.User{
		TelegramID:  42,
		Username:    "old_alice",
		DisplayName: "Old Alice",
		Role:        entity.RoleViewer,
		IsActive:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	repo.users[42] = existing

	store := &memStorage{users: repo}
	// Sender now has a new username.
	sender := &telebot.User{ID: 42, Username: "new_alice", FirstName: "New", LastName: "Alice"}
	ctx := newFakeCtx(sender, sender.ID)

	called, _ := runMiddleware(ctx, store, defaultTelegramCfg())
	require.True(t, called)

	updated, err := repo.GetByTelegramID(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, "new_alice", updated.Username, "username must be updated on change")
	assert.Equal(t, "New Alice", updated.DisplayName, "display name must be updated on change")
}

func TestAuth_ExistingUser_InjectedIntoContext(t *testing.T) {
	t.Parallel()

	repo := newMemUserRepo()
	repo.users[42] = &entity.User{
		TelegramID: 42,
		Username:   "alice",
		Role:       entity.RoleAdmin,
		IsActive:   true,
	}

	store := &memStorage{users: repo}
	sender := &telebot.User{ID: 42, Username: "alice", FirstName: "Alice"}
	ctx := newFakeCtx(sender, sender.ID)

	var capturedUser *entity.User
	handler := func(c telebot.Context) error {
		capturedUser = GetUser(c)
		return nil
	}
	mw := buildAuth(store, defaultTelegramCfg())
	_ = mw(handler)(ctx)

	require.NotNil(t, capturedUser, "user must be injected into context")
	assert.Equal(t, int64(42), capturedUser.TelegramID)
	assert.Equal(t, entity.RoleAdmin, capturedUser.Role)
}

func TestAuth_MultipleAdminIDs(t *testing.T) {
	t.Parallel()

	adminIDs := []int64{10, 20, 30}
	cfg := config.TelegramConfig{
		Token:    "test-token",
		AdminIDs: adminIDs,
	}

	for _, adminID := range adminIDs {
		adminID := adminID
		t.Run("admin id registered as admin", func(t *testing.T) {
			t.Parallel()
			repo := newMemUserRepo()
			store := &memStorage{users: repo}
			sender := &telebot.User{ID: adminID, Username: "admin", FirstName: "Admin"}
			ctx := newFakeCtx(sender, sender.ID)
			runMiddleware(ctx, store, cfg)

			user, err := repo.GetByTelegramID(context.Background(), adminID)
			require.NoError(t, err)
			assert.Equal(t, entity.RoleAdmin, user.Role)
		})
	}
}
