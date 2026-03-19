package handler_test

import (
	"context"
	"strings"
	"testing"

	"github.com/d9042n/telekube/internal/bot/handler"
	"github.com/d9042n/telekube/internal/bot/keyboard"
	"github.com/d9042n/telekube/internal/cluster"
	"github.com/d9042n/telekube/internal/entity"
	"github.com/d9042n/telekube/internal/module"
	"github.com/d9042n/telekube/internal/rbac"
	"github.com/d9042n/telekube/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gopkg.in/telebot.v3"
)

// ─── Minimal mock telebot.Context ────────────────────────────────────────────
// We embed a nil telebot.Context and only override the methods our handlers use.
// Any unimplemented method will panic if called — which is useful to detect
// unexpected handler behaviour in tests.

type fakeCtx struct {
	telebot.Context                    // Embed nil interface — panics if truly unimplemented method is called
	store          map[string]interface{}
	messages       []string
	responds       []string
}

func newFakeCtx(user *entity.User) *fakeCtx {
	f := &fakeCtx{store: make(map[string]interface{})}
	if user != nil {
		f.store["user"] = user
	}
	return f
}

func (f *fakeCtx) Get(key string) interface{} {
	return f.store[key]
}

func (f *fakeCtx) Set(key string, val interface{}) {
	f.store[key] = val
}

func (f *fakeCtx) Send(what interface{}, _ ...interface{}) error {
	if s, ok := what.(string); ok {
		f.messages = append(f.messages, s)
	}
	return nil
}

func (f *fakeCtx) Respond(responses ...*telebot.CallbackResponse) error {
	for _, r := range responses {
		if r != nil {
			f.responds = append(f.responds, r.Text)
		}
	}
	return nil
}

func (f *fakeCtx) Callback() *telebot.Callback { return nil }
func (f *fakeCtx) Sender() *telebot.User       { return nil }
func (f *fakeCtx) Bot() *telebot.Bot           { return nil }

// fakeCtxWithCallback extends fakeCtx with a real Callback() value.
// Used for ClusterSelect handler tests.
type fakeCtxWithCallback struct {
	*fakeCtx
	cb *telebot.Callback
}

func newCallbackCtx(user *entity.User, data string) *fakeCtxWithCallback {
	return &fakeCtxWithCallback{
		fakeCtx: newFakeCtx(user),
		cb: &telebot.Callback{Data: data},
	}
}

func (f *fakeCtxWithCallback) Callback() *telebot.Callback { return f.cb }

// ─── Fake RBAC that satisfies rbac.Engine ────────────────────────────────────

type testRBAC struct {
	mock.Mock
	role    string
	allowed map[string]bool
}

func newAllowRBAC(role string) *testRBAC {
	return &testRBAC{role: role, allowed: map[string]bool{"*": true}}
}

func newDenyRBAC() *testRBAC {
	return &testRBAC{role: entity.RoleViewer, allowed: map[string]bool{}}
}

func newSelectiveRBAC(role string, perms ...string) *testRBAC {
	allowed := make(map[string]bool)
	for _, p := range perms {
		allowed[p] = true
	}
	return &testRBAC{role: role, allowed: allowed}
}

func (r *testRBAC) HasPermission(_ context.Context, _ int64, perm string) (bool, error) {
	if r.allowed["*"] {
		return true, nil
	}
	return r.allowed[perm], nil
}

func (r *testRBAC) GetRole(_ context.Context, _ int64) (string, error) {
	return r.role, nil
}

func (r *testRBAC) Authorize(_ context.Context, _ int64, _ rbac.PermissionRequest) (bool, error) {
	return r.allowed["*"], nil
}

func (r *testRBAC) SetRole(_ context.Context, _ int64, _ string) error { return nil }
func (r *testRBAC) RolePermissions(_ string) []string                  { return nil }
func (r *testRBAC) IsSuperAdmin(_ int64) bool                          { return false }
func (r *testRBAC) Roles() []entity.Role                               { return nil }
func (r *testRBAC) CreateRole(_ context.Context, _ *entity.Role) error { return nil }
func (r *testRBAC) ListRoles(_ context.Context) ([]entity.Role, error) { return nil, nil }
func (r *testRBAC) AssignRole(_ context.Context, _ *entity.UserRoleBinding) error {
	return nil
}
func (r *testRBAC) RevokeRole(_ context.Context, _ int64, _ string) error { return nil }
func (r *testRBAC) ListUserBindings(_ context.Context, _ int64) ([]entity.UserRoleBinding, error) {
	return nil, nil
}
func (r *testRBAC) ListAllBindings(_ context.Context) ([]entity.UserRoleBinding, error) {
	return nil, nil
}

// ─── Fake CommandRegistry ──────────────────────────────────────────────────────

type testRegistry struct {
	cmds []module.CommandInfo
}

func (r *testRegistry) AllCommands() []module.CommandInfo { return r.cmds }

// ─── helpers ──────────────────────────────────────────────────────────────────

func testUser(id int64, username, role string) *entity.User {
	return &entity.User{
		TelegramID:  id,
		Username:    username,
		DisplayName: username,
		Role:        role,
		IsActive:    true,
	}
}

func newUserCtx(mgr cluster.Manager) *cluster.UserContext {
	return cluster.NewUserContext(mgr)
}

// ─── Start handler tests ───────────────────────────────────────────────────────

func TestStart_UserNotFound_SendsWarning(t *testing.T) {
	t.Parallel()

	mgr := testutil.NewFakeClusterManager()
	rb := newAllowRBAC(entity.RoleViewer)
	kb := keyboard.NewBuilder()

	ctx := newFakeCtx(nil) // no user in context
	h := handler.Start(mgr, newUserCtx(mgr), rb, kb)
	err := h(ctx)

	assert.NoError(t, err)
	assert.Len(t, ctx.messages, 1)
	assert.Contains(t, ctx.messages[0], "Could not identify you")
}

func TestStart_AdminUser_ShowsAdminRole(t *testing.T) {
	t.Parallel()

	mgr := testutil.NewFakeClusterManager()
	rb := newAllowRBAC(entity.RoleAdmin)
	kb := keyboard.NewBuilder()

	user := testUser(1, "alice", entity.RoleAdmin)
	ctx := newFakeCtx(user)

	h := handler.Start(mgr, newUserCtx(mgr), rb, kb)
	err := h(ctx)

	assert.NoError(t, err)
	assert.Len(t, ctx.messages, 1)
	assert.Contains(t, ctx.messages[0], "admin",
		"welcome message should display the role")
}

func TestStart_ViewerUser_ShowsViewerRole(t *testing.T) {
	t.Parallel()

	mgr := testutil.NewFakeClusterManager()
	rb := newAllowRBAC(entity.RoleViewer)
	kb := keyboard.NewBuilder()

	user := testUser(2, "bob", entity.RoleViewer)
	ctx := newFakeCtx(user)

	h := handler.Start(mgr, newUserCtx(mgr), rb, kb)
	err := h(ctx)

	assert.NoError(t, err)
	assert.Contains(t, ctx.messages[0], "viewer")
}

func TestStart_WithCurrentCluster_ShowsClusterName(t *testing.T) {
	t.Parallel()

	mgr := testutil.NewFakeClusterManagerNamed("staging")
	uctx := newUserCtx(mgr)
	uctx.SetCluster(3, "staging")

	rb := newAllowRBAC(entity.RoleOperator)
	kb := keyboard.NewBuilder()

	user := testUser(3, "charlie", entity.RoleOperator)
	ctx := newFakeCtx(user)

	h := handler.Start(mgr, uctx, rb, kb)
	err := h(ctx)

	assert.NoError(t, err)
	assert.Contains(t, ctx.messages[0], "staging",
		"selected cluster should appear in welcome message")
}

func TestStart_AlwaysSendsExactlyOneMessage(t *testing.T) {
	t.Parallel()

	mgr := testutil.NewFakeClusterManager()
	rb := newAllowRBAC(entity.RoleViewer)
	kb := keyboard.NewBuilder()

	user := testUser(4, "diana", entity.RoleViewer)
	ctx := newFakeCtx(user)

	h := handler.Start(mgr, newUserCtx(mgr), rb, kb)
	err := h(ctx)

	assert.NoError(t, err)
	assert.Len(t, ctx.messages, 1, "Start should produce exactly one message per call")
}

// ─── Help handler tests ────────────────────────────────────────────────────────

func TestHelp_NoUser_SendsWarning(t *testing.T) {
	t.Parallel()

	reg := &testRegistry{}
	rb := newAllowRBAC(entity.RoleViewer)

	ctx := newFakeCtx(nil)
	h := handler.Help(reg, rb)
	err := h(ctx)

	assert.NoError(t, err)
	assert.Contains(t, ctx.messages[0], "Could not identify you")
}

func TestHelp_AdminSeesBothCommands(t *testing.T) {
	t.Parallel()

	reg := &testRegistry{
		cmds: []module.CommandInfo{
			{Command: "/pods", Description: "List pods", Permission: "kubernetes.pods.list"},
			{Command: "/audit", Description: "Audit log", Permission: "audit.view"},
		},
	}
	rb := newAllowRBAC(entity.RoleAdmin)
	user := testUser(1, "admin", entity.RoleAdmin)
	ctx := newFakeCtx(user)

	h := handler.Help(reg, rb)
	err := h(ctx)

	assert.NoError(t, err)
	msg := ctx.messages[0]
	assert.Contains(t, msg, "/pods")
	assert.Contains(t, msg, "/audit")
}

func TestHelp_ViewerOnlySeesPodCommand(t *testing.T) {
	t.Parallel()

	reg := &testRegistry{
		cmds: []module.CommandInfo{
			{Command: "/pods", Description: "List pods", Permission: "kubernetes.pods.list"},
			{Command: "/audit", Description: "Audit log", Permission: "audit.view"},
		},
	}

	// Viewer can see pods but not audit
	rb := newSelectiveRBAC(entity.RoleViewer, "kubernetes.pods.list")
	user := testUser(2, "viewer", entity.RoleViewer)
	ctx := newFakeCtx(user)

	h := handler.Help(reg, rb)
	err := h(ctx)

	assert.NoError(t, err)
	msg := ctx.messages[0]
	assert.Contains(t, msg, "/pods", "viewer should see pods command")
	assert.NotContains(t, msg, "/audit", "viewer should NOT see audit command")
}

func TestHelp_DenyAllViewer_SeesNoPermissionedCommands(t *testing.T) {
	t.Parallel()

	reg := &testRegistry{
		cmds: []module.CommandInfo{
			{Command: "/pods", Description: "List pods", Permission: "kubernetes.pods.list"},
		},
	}

	rb := newDenyRBAC()
	user := testUser(3, "readonly", entity.RoleViewer)
	ctx := newFakeCtx(user)

	h := handler.Help(reg, rb)
	err := h(ctx)

	assert.NoError(t, err)
	assert.NotContains(t, ctx.messages[0], "/pods",
		"deny-all RBAC should hide all permissioned commands")
}

func TestHelp_CommandWithNoPermission_AlwaysVisible(t *testing.T) {
	t.Parallel()

	reg := &testRegistry{
		cmds: []module.CommandInfo{
			{Command: "/status", Description: "Cluster status", Permission: ""},
		},
	}

	rb := newDenyRBAC() // deny everything
	user := testUser(4, "guest", entity.RoleViewer)
	ctx := newFakeCtx(user)

	h := handler.Help(reg, rb)
	err := h(ctx)

	assert.NoError(t, err)
	assert.Contains(t, ctx.messages[0], "/status",
		"commands with empty permission string should always be visible")
}

// ─── Clusters handler tests ────────────────────────────────────────────────────

func TestClusters_NoUser_SendsWarning(t *testing.T) {
	t.Parallel()

	mgr := testutil.NewFakeClusterManager()
	kb := keyboard.NewBuilder()

	ctx := newFakeCtx(nil)
	h := handler.Clusters(mgr, newUserCtx(mgr), kb)
	err := h(ctx)

	assert.NoError(t, err)
	assert.Contains(t, ctx.messages[0], "Could not identify you")
}

func TestClusters_ListsAllClusters(t *testing.T) {
	t.Parallel()

	mgr := testutil.NewFakeClusterManagerNamed("prod")
	kb := keyboard.NewBuilder()

	user := testUser(1, "alice", entity.RoleAdmin)
	ctx := newFakeCtx(user)

	h := handler.Clusters(mgr, newUserCtx(mgr), kb)
	err := h(ctx)

	assert.NoError(t, err)
	assert.Contains(t, ctx.messages[0], "prod")
}

func TestClusters_CurrentClusterMarked(t *testing.T) {
	t.Parallel()

	mgr := testutil.NewFakeClusterManagerNamed("prod")
	uctx := newUserCtx(mgr)
	uctx.SetCluster(1, "prod")

	kb := keyboard.NewBuilder()
	user := testUser(1, "alice", entity.RoleAdmin)
	ctx := newFakeCtx(user)

	h := handler.Clusters(mgr, uctx, kb)
	err := h(ctx)

	assert.NoError(t, err)
	msg := ctx.messages[0]
	assert.True(t, strings.Contains(msg, "← current"),
		"active cluster should be marked with ← current: %s", msg)
}

// ─── ClusterSelect handler tests ──────────────────────────────────────────────

func TestClusterSelect_NoUser_RespondsWithError(t *testing.T) {
	t.Parallel()

	mgr := testutil.NewFakeClusterManager()
	ctx := newCallbackCtx(nil, "prod")

	h := handler.ClusterSelect(mgr, newUserCtx(mgr))
	err := h(ctx)

	assert.NoError(t, err)
	assert.Len(t, ctx.responds, 1)
	assert.Contains(t, ctx.responds[0], "Error")
}

func TestClusterSelect_EmptyCallbackData_RespondsNoCluster(t *testing.T) {
	t.Parallel()

	mgr := testutil.NewFakeClusterManager()
	user := testUser(1, "alice", entity.RoleAdmin)
	ctx := newCallbackCtx(user, "") // empty data

	h := handler.ClusterSelect(mgr, newUserCtx(mgr))
	err := h(ctx)

	assert.NoError(t, err)
	assert.Contains(t, ctx.responds[0], "No cluster selected")
}

func TestClusterSelect_ClusterNotFound_RespondsError(t *testing.T) {
	t.Parallel()

	mgr := testutil.NewFakeClusterManager() // no clusters registered
	user := testUser(1, "alice", entity.RoleAdmin)
	ctx := newCallbackCtx(user, "nonexistent-cluster")

	h := handler.ClusterSelect(mgr, newUserCtx(mgr))
	err := h(ctx)

	assert.NoError(t, err)
	assert.Contains(t, ctx.responds[0], "Cluster not found")
}

func TestClusterSelect_ValidCluster_SetsAndResponds(t *testing.T) {
	t.Parallel()

	mgr := testutil.NewFakeClusterManagerNamed("prod")
	uctx := newUserCtx(mgr)

	user := testUser(1, "alice", entity.RoleAdmin)
	ctx := newCallbackCtx(user, "prod")

	h := handler.ClusterSelect(mgr, uctx)
	err := h(ctx)

	assert.NoError(t, err)
	assert.Len(t, ctx.responds, 1)
	assert.Contains(t, ctx.responds[0], "Switched to")

	// Verify the cluster was stored in the user context
	clusterName := uctx.GetCluster(1)
	assert.Equal(t, "prod", clusterName)
}

