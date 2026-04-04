package types

// Fiber c.Locals keys — used as plain strings to avoid interface{} type-assertion pain.
const (
	CtxUserID   = "userID"
	CtxTenantID = "tenantID"
	CtxRole     = "role"
	CtxTx       = "tx" // holds pgx.Tx for the current request transaction
)

// roleWeight maps each role to a numeric rank for hierarchy comparisons.
var roleWeight = map[string]int{
	"viewer": 1,
	"member": 2,
	"admin":  3,
	"owner":  4,
}

// RoleAtLeast returns true if `have` is at least as privileged as `need`.
// e.g. RoleAtLeast("admin", "member") == true
func RoleAtLeast(have, need string) bool {
	return roleWeight[have] >= roleWeight[need]
}
