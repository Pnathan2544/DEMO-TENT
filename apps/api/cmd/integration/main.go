// Integration tests for Milestone A + B.
//
// Usage:
//
//	go run ./cmd/integration
//	DATABASE_URL=postgres://saas_app:saas_secret@localhost:5432/saas_db go run ./cmd/integration
//
// DATABASE_URL is optional; omitting it skips invite-accept happy path and related DB checks.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ── colours ───────────────────────────────────────────────────────────────────

const (
	clGreen  = "\033[32m"
	clRed    = "\033[31m"
	clYellow = "\033[33m"
	clCyan   = "\033[36m"
	clReset  = "\033[0m"
)

// ── test harness ──────────────────────────────────────────────────────────────

type T struct {
	base string
	pass int
	fail int
	skip int
}

func (t *T) check(desc, expected, actual string) {
	if actual == expected {
		fmt.Printf("%sPASS%s %s\n", clGreen, clReset, desc)
		t.pass++
	} else {
		fmt.Printf("%sFAIL%s %s — expected %s, got %s\n", clRed, clReset, desc, expected, actual)
		t.fail++
	}
}

func (t *T) checkb(desc string, ok bool) {
	if ok {
		fmt.Printf("%sPASS%s %s\n", clGreen, clReset, desc)
		t.pass++
	} else {
		fmt.Printf("%sFAIL%s %s\n", clRed, clReset, desc)
		t.fail++
	}
}

func (t *T) skipf(desc string) {
	fmt.Printf("%sSKIP%s %s\n", clYellow, clReset, desc)
	t.skip++
}

func header(s string) {
	fmt.Printf("\n%s--- %s ---%s\n", clCyan, s, clReset)
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func newClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, Timeout: 10 * time.Second}
}

type R struct {
	Status int
	raw    []byte
	obj    map[string]any // non-nil only when response is a JSON object
}

func (r R) code() string { return fmt.Sprintf("%d", r.Status) }

func (r R) str(key string) string {
	s, _ := r.obj[key].(string)
	return s
}

func (r R) nested(keys ...string) string {
	m := r.obj
	for i, k := range keys {
		v, ok := m[k]
		if !ok {
			return ""
		}
		if i == len(keys)-1 {
			s, _ := v.(string)
			return s
		}
		m, ok = v.(map[string]any)
		if !ok {
			return ""
		}
	}
	return ""
}

func (r R) arr() []any {
	var a []any
	json.Unmarshal(r.raw, &a)
	return a
}

func (r R) hasAction(action string) bool {
	for _, e := range r.arr() {
		m, ok := e.(map[string]any)
		if ok && m["action"] == action {
			return true
		}
	}
	return false
}

func do(client *http.Client, method, url string, body any) R {
	var br io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		br = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, br)
	if err != nil {
		return R{}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return R{Status: 0}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var obj map[string]any
	json.Unmarshal(raw, &obj)
	if obj == nil {
		obj = map[string]any{}
	}
	return R{Status: resp.StatusCode, raw: raw, obj: obj}
}

func (t *T) GET(c *http.Client, path string) R {
	return do(c, "GET", t.base+path, nil)
}
func (t *T) POST(c *http.Client, path string, body any) R {
	return do(c, "POST", t.base+path, body)
}
func (t *T) PUT(c *http.Client, path string, body any) R {
	return do(c, "PUT", t.base+path, body)
}
func (t *T) PATCH(c *http.Client, path string, body any) R {
	return do(c, "PATCH", t.base+path, body)
}
func (t *T) DELETE(c *http.Client, path string) R {
	return do(c, "DELETE", t.base+path, nil)
}

// ── misc helpers ──────────────────────────────────────────────────────────────

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func m(kv ...any) map[string]any {
	out := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		out[kv[i].(string)] = kv[i+1]
	}
	return out
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	t := &T{base: getenv("BASE_URL", "http://localhost:8080")}
	dbURL := os.Getenv("DATABASE_URL")

	// Pre-flight: confirm server is reachable.
	probe, err := http.Get(t.base + "/api/v1/auth/login") //nolint:noctx
	if err != nil {
		fmt.Println("ERROR: Cannot reach", t.base, "— is the server running?")
		fmt.Println("       cd apps/api && go run ./cmd/api")
		os.Exit(1)
	}
	probe.Body.Close()

	fmt.Printf("\n%s=== Milestone A + B Integration Tests ===%s\n", clCyan, clReset)
	fmt.Println("Base URL :", t.base)
	fmt.Println("DB access:", dbURL != "")

	ts := fmt.Sprintf("%d", time.Now().UnixNano())
	emailA := "alice_" + ts + "@example.com"
	emailB := "bob_" + ts + "@example.com"
	tenantA := "Acme Corp " + ts
	tenantB := "Bobs LLC " + ts

	cA := newClient()    // Alice
	cB := newClient()    // Bob
	cAnon := newClient() // unauthenticated

	var userAID, tenantAID, userBID, tenantBID, projectID, inviteID string

	// ══════════════════════════════════════════════════════════════════════════
	header("[A] Milestone A Regression")
	// ══════════════════════════════════════════════════════════════════════════

	r := t.POST(cA, "/api/v1/auth/register", m("email", emailA, "name", "Alice", "password", "password123", "tenantName", tenantA))
	t.check("A.1  Register User A → 201", "201", r.code())
	userAID = r.nested("user", "id")
	tenantAID = r.nested("tenant", "id")

	r = t.POST(newClient(), "/api/v1/auth/register", m("email", emailA, "name", "X", "password", "password123", "tenantName", "Other Corp"))
	t.check("A.2  Duplicate email → 409", "409", r.code())

	r = t.POST(newClient(), "/api/v1/auth/register", m("email", "other_"+ts+"@example.com", "name", "X", "password", "pass123456", "tenantName", tenantA))
	t.check("A.3  Duplicate tenant name → 409", "409", r.code())

	r = t.POST(newClient(), "/api/v1/auth/register", m("email", "x@x.com"))
	t.check("A.4  Register missing fields → 400", "400", r.code())

	// Re-login so cA has a fresh token (register set one; test login separately).
	cA = newClient()
	r = t.POST(cA, "/api/v1/auth/login", m("email", emailA, "password", "password123"))
	t.check("A.5  Login → 200", "200", r.code())

	r = t.POST(newClient(), "/api/v1/auth/login", m("email", emailA, "password", "wrong"))
	t.check("A.6  Wrong password → 401", "401", r.code())

	r = t.GET(cA, "/api/v1/me/")
	t.check("A.7  GET /me → 200", "200", r.code())
	t.checkb("A.7a /me.tenantId matches", r.str("tenantId") == tenantAID)

	r = t.GET(cAnon, "/api/v1/me/")
	t.check("A.8  GET /me unauthenticated → 401", "401", r.code())

	r = t.PUT(cA, "/api/v1/me/", m("name", "Alice Smith"))
	t.check("A.9  PUT /me → 204", "204", r.code())

	r = t.PUT(cA, "/api/v1/me/password", m("current", "wrongpass", "new", "newpass456"))
	t.check("A.10 PUT /me/password wrong current → 401", "401", r.code())

	r = t.PUT(cA, "/api/v1/me/password", m("current", "password123", "new", "newpass456"))
	t.check("A.11 PUT /me/password → 204", "204", r.code())

	r = t.PUT(cA, "/api/v1/me/password", m("current", "newpass456", "new", "password123"))
	t.check("A.11b Restore password → 204", "204", r.code())

	r = t.POST(cA, "/api/v1/projects", m("name", "Alpha Project", "description", "Test"))
	t.check("A.12 Create project → 201", "201", r.code())
	projectID = r.str("id")

	r = t.GET(cA, "/api/v1/projects/")
	t.check("A.13 List projects → 200", "200", r.code())

	r = t.GET(cA, "/api/v1/projects/"+projectID)
	t.check("A.14 Get project → 200", "200", r.code())

	r = t.POST(cA, "/api/v1/auth/refresh", nil)
	t.check("A.15 Refresh token → 200", "200", r.code())

	// ══════════════════════════════════════════════════════════════════════════
	header("[B1] Tenant Switching")
	// ══════════════════════════════════════════════════════════════════════════

	r = t.POST(cB, "/api/v1/auth/register", m("email", emailB, "name", "Bob", "password", "password123", "tenantName", tenantB))
	t.check("B1.0 Register User B → 201", "201", r.code())
	userBID = r.nested("user", "id")
	tenantBID = r.nested("tenant", "id")

	r = t.GET(cA, "/api/v1/auth/tenants")
	t.check("B1.1 GET /auth/tenants → 200", "200", r.code())
	t.checkb("B1.1a Alice has 1 tenant", len(r.arr()) == 1)

	r = t.POST(cA, "/api/v1/auth/switch", m("tenantId", tenantBID))
	t.check("B1.2 Switch non-member tenant → 403", "403", r.code())

	r = t.POST(cA, "/api/v1/auth/switch", m())
	t.check("B1.3 Switch missing tenantId → 400", "400", r.code())

	r = t.POST(cAnon, "/api/v1/auth/switch", m("tenantId", tenantBID))
	t.check("B1.4 Switch unauthenticated → 401", "401", r.code())

	// ══════════════════════════════════════════════════════════════════════════
	header("[B2] Invite Management")
	// ══════════════════════════════════════════════════════════════════════════

	r = t.POST(cA, "/api/v1/invites", m("email", emailB, "role", "member"))
	t.check("B2.1 POST /invites → 201", "201", r.code())
	inviteID = r.str("id")

	r = t.POST(cA, "/api/v1/invites", m("email", emailB, "role", "member"))
	t.check("B2.2 Duplicate invite → 409", "409", r.code())

	r = t.POST(cA, "/api/v1/invites", m("email", emailA, "role", "member"))
	t.check("B2.3 Invite existing member → 409", "409", r.code())

	r = t.POST(cA, "/api/v1/invites", m("email", "s_"+ts+"@example.com", "role", "owner"))
	t.check("B2.4 role=owner → 400", "400", r.code())

	r = t.POST(cA, "/api/v1/invites", m("email", "s_"+ts+"@example.com", "role", "superadmin"))
	t.check("B2.5 Invalid role → 400", "400", r.code())

	r = t.GET(cA, "/api/v1/invites/")
	t.check("B2.6 GET /invites as admin → 200", "200", r.code())

	r = t.GET(cAnon, "/api/v1/invites/")
	t.check("B2.7 GET /invites unauthenticated → 401", "401", r.code())

	r = t.DELETE(cA, "/api/v1/invites/"+inviteID)
	t.check("B2.8 DELETE /invites/:id → 204", "204", r.code())

	r = t.DELETE(cA, "/api/v1/invites/"+inviteID)
	t.check("B2.9 DELETE non-existent → 404", "404", r.code())

	r = t.POST(cA, "/api/v1/invites", m("email", emailB, "role", "member"))
	t.check("B2.10 Re-create invite → 201", "201", r.code())
	inviteID = r.str("id")

	// ══════════════════════════════════════════════════════════════════════════
	header("[B3] Accept Invite")
	// ══════════════════════════════════════════════════════════════════════════

	r = t.POST(cB, "/api/v1/invites/accept", m("token", "bogus-token-not-real"))
	t.check("B3.1 Accept bogus token → 404", "404", r.code())

	r = t.POST(cB, "/api/v1/invites/accept", m())
	t.check("B3.2 Accept missing token → 400", "400", r.code())

	r = t.POST(cAnon, "/api/v1/invites/accept", m("token", "abc"))
	t.check("B3.3 Accept unauthenticated → 401", "401", r.code())

	r = t.GET(cB, "/api/v1/me/invites")
	t.check("B3.4 GET /me/invites → 200", "200", r.code())
	t.checkb("B3.4a Bob has 1 pending invite", len(r.arr()) == 1)

	if dbURL != "" {
		ctx := context.Background()
		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			fmt.Println("WARN: DB connect failed:", err)
			t.skipf("B3.5-B3.15 (DB unavailable)")
		} else {
			defer pool.Close()
			doDBTests(t, ctx, pool, cA, cB, cAnon, tenantAID, userAID, userBID, emailB, inviteID, ts)
		}
	} else {
		for _, s := range []string{
			"B3.5 email mismatch", "B3.6 happy path", "B3.7 consumed token",
			"B3.8 switch after join", "B3.9 /me tenantId", "B3.10 2 tenants",
			"B3.11 member !GET /invites", "B3.12 member !POST /invites",
			"B3.13 member GET /members", "B3.14 member !PATCH /members",
			"B3.15 RLS project isolation",
		} {
			t.skipf(s + " (set DATABASE_URL)")
		}
	}

	// ══════════════════════════════════════════════════════════════════════════
	header("[B4] Member Management")
	// ══════════════════════════════════════════════════════════════════════════

	r = t.GET(cA, "/api/v1/members/")
	t.check("B4.1 GET /members as owner → 200", "200", r.code())

	r = t.PATCH(cA, "/api/v1/members/"+userAID, m("role", "member"))
	t.check("B4.2 Change own role → 403", "403", r.code())

	r = t.PATCH(cA, "/api/v1/members/"+userBID, m("role", "superadmin"))
	t.check("B4.3 Invalid role → 400", "400", r.code())

	r = t.DELETE(cA, "/api/v1/members/"+userAID)
	t.check("B4.4 Remove last owner → 409", "409", r.code())

	if dbURL != "" {
		// Bob is now in Tenant A (added in B3 above).
		r = t.PATCH(cA, "/api/v1/members/"+userBID, m("role", "viewer"))
		t.check("B4.5 Demote Bob to viewer → 204", "204", r.code())

		r = t.PATCH(cA, "/api/v1/members/"+userBID, m("role", "admin"))
		t.check("B4.6 Promote Bob to admin → 204", "204", r.code())

		r = t.PATCH(cA, "/api/v1/members/"+userAID, m("role", "admin"))
		t.check("B4.7 Cannot change own role → 403", "403", r.code())

		r = t.DELETE(cA, "/api/v1/members/"+userBID)
		t.check("B4.8 Owner removes admin → 204", "204", r.code())

		r = t.GET(cA, "/api/v1/members/")
		t.check("B4.9 GET /members → 200", "200", r.code())
		t.checkb("B4.9a Back to 1 member", len(r.arr()) == 1)
	} else {
		t.skipf("B4.5-B4.9 (set DATABASE_URL)")
	}

	// ══════════════════════════════════════════════════════════════════════════
	header("[B5] Audit Log")
	// ══════════════════════════════════════════════════════════════════════════

	r = t.GET(cA, "/api/v1/audit")
	t.check("B5.1 GET /audit as owner → 200", "200", r.code())

	if dbURL != "" {
		t.checkb("B5.1a Audit log has events", len(r.arr()) > 0)

		cBFresh := newClient()
		r = t.POST(cBFresh, "/api/v1/auth/login", m("email", emailB, "password", "password123"))
		t.check("B5.2a Bob re-login (Tenant B) → 200", "200", r.code())

		r = t.GET(cBFresh, "/api/v1/audit")
		t.check("B5.2b GET /audit Tenant B → 200", "200", r.code())
		t.checkb("B5.2c No cross-tenant audit bleed", len(r.arr()) == 0)

		r = t.GET(cA, "/api/v1/audit")
		t.check("B5.3 Re-fetch Tenant A audit → 200", "200", r.code())
		t.checkb("B5.3a invite_created present", r.hasAction("invite_created"))
		t.checkb("B5.3b invite_revoked present", r.hasAction("invite_revoked"))
		t.checkb("B5.3c role_changed present", r.hasAction("role_changed"))
		t.checkb("B5.3d member_removed present", r.hasAction("member_removed"))
		t.checkb("B5.3e password_changed present", r.hasAction("password_changed"))

		r = t.GET(cAnon, "/api/v1/audit")
		t.check("B5.4 GET /audit unauthenticated → 401", "401", r.code())
	} else {
		t.skipf("B5.1a-B5.4 (set DATABASE_URL)")
	}

	// ══════════════════════════════════════════════════════════════════════════
	header("[A] Logout + Session Expiry Regression")
	// ══════════════════════════════════════════════════════════════════════════

	r = t.POST(cA, "/api/v1/auth/logout", nil)
	t.check("A.16 Logout → 204", "204", r.code())

	r = t.GET(cA, "/api/v1/me/")
	t.check("A.17 /me after logout → 401", "401", r.code())

	r = t.POST(cA, "/api/v1/auth/refresh", nil)
	t.check("A.18 Refresh after logout → 401", "401", r.code())

	r = t.GET(cA, "/api/v1/projects/")
	t.check("A.19 /projects after logout → 401", "401", r.code())

	// ── summary ───────────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Println("════════════════════════════════════════════")
	fmt.Printf("  %s%3d passed%s   %s%3d failed%s   %s%3d skipped%s\n",
		clGreen, t.pass, clReset,
		clRed, t.fail, clReset,
		clYellow, t.skip, clReset)
	fmt.Println("════════════════════════════════════════════")
	if t.fail > 0 {
		os.Exit(1)
	}
}

// doDBTests runs the tests that require direct DB access (B3 happy path, etc.).
func doDBTests(t *T, ctx context.Context, pool *pgxpool.Pool,
	cA, cB, cAnon *http.Client,
	tenantAID, userAID, userBID, emailB, inviteID, ts string,
) {
	knownRaw := "go-test-token-" + ts
	knownHash := hashToken(knownRaw)

	// Replace the real invite with one we know the raw token for.
	pool.Exec(ctx, "DELETE FROM invites WHERE id = $1", inviteID)
	future := time.Now().Add(time.Hour)
	pool.Exec(ctx, `INSERT INTO invites (tenant_id, invited_by, email, role, token_hash, expires_at)
		VALUES ($1, $2, $3, 'member', $4, $5)`,
		tenantAID, userAID, emailB, knownHash, future)

	r := t.POST(cA, "/api/v1/invites/accept", m("token", knownRaw))
	t.check("B3.5 Accept with wrong email → 403", "403", r.code())

	r = t.POST(cB, "/api/v1/invites/accept", m("token", knownRaw))
	t.check("B3.6 Accept invite happy path → 200", "200", r.code())
	t.checkb("B3.6a role=member returned", r.str("role") == "member")

	r = t.POST(cB, "/api/v1/invites/accept", m("token", knownRaw))
	t.check("B3.7 Accept already-consumed → 404", "404", r.code())

	r = t.POST(cB, "/api/v1/auth/switch", m("tenantId", tenantAID))
	t.check("B3.8 Bob switches to Tenant A → 200", "200", r.code())
	t.checkb("B3.8a role=member after switch", r.str("role") == "member")

	r = t.GET(cB, "/api/v1/me/")
	t.check("B3.9 /me after switch → 200", "200", r.code())
	t.checkb("B3.9a tenantId updated", r.str("tenantId") == tenantAID)

	r = t.GET(cB, "/api/v1/auth/tenants")
	t.check("B3.10 GET /auth/tenants (Bob) → 200", "200", r.code())
	t.checkb("B3.10a Bob has 2 tenants", len(r.arr()) == 2)

	r = t.GET(cB, "/api/v1/invites/")
	t.check("B3.11 Member cannot GET /invites → 403", "403", r.code())

	r = t.POST(cB, "/api/v1/invites", m("email", "x_"+ts+"@example.com", "role", "viewer"))
	t.check("B3.12 Member cannot POST /invites → 403", "403", r.code())

	r = t.GET(cB, "/api/v1/members/")
	t.check("B3.13 Member can GET /members → 200", "200", r.code())

	r = t.PATCH(cB, "/api/v1/members/"+userAID, m("role", "viewer"))
	t.check("B3.14 Member cannot PATCH /members → 403", "403", r.code())

	r = t.GET(cB, "/api/v1/projects/")
	t.check("B3.15 RLS Bob sees Tenant A projects → 200", "200", r.code())
	t.checkb("B3.15a sees Alice's 1 project", len(r.arr()) == 1)

	_ = cAnon // used in main, not here
	_ = userBID
}
