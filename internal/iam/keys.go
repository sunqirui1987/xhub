package iam

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"xorm.io/builder"
	"xorm.io/xorm"

	"github.com/sunqirui1987/xhub/internal/logx"
)

// Key is a virtual key. A personal key belongs to a user inside a team; a
// service key belongs to the team (or one of its projects) with no owner.
type Key struct {
	ID             string     `xorm:"pk 'id'" json:"id"`
	TokenHash      string     `xorm:"'token_hash'" json:"-"`
	KeyPrefix      string     `xorm:"'key_prefix'" json:"key_prefix"`
	OwnerType      string     `xorm:"'owner_type'" json:"owner_type"`
	UserID         *string    `xorm:"'user_id'" json:"user_id"`
	TeamID         string     `xorm:"'team_id'" json:"team_id"`
	ProjectID      *string    `xorm:"'project_id'" json:"project_id"`
	CreatedBy      *string    `xorm:"'created_by'" json:"created_by"`
	Name           string     `xorm:"'name'" json:"name"`
	Models         []string   `xorm:"json 'models'" json:"models"`
	MaxBudget      *float64   `xorm:"'max_budget'" json:"max_budget"`
	Spend          float64    `xorm:"'spend'" json:"spend"`
	TPMLimit       *int       `xorm:"'tpm_limit'" json:"tpm_limit"`
	RPMLimit       *int       `xorm:"'rpm_limit'" json:"rpm_limit"`
	Status         string     `xorm:"'status'" json:"status"`
	ExpiresAt      *time.Time `xorm:"'expires_at'" json:"expires_at"`
	LastUsedAt     *time.Time `xorm:"'last_used_at'" json:"last_used_at"`
	CreatedAt      time.Time  `xorm:"created 'created_at'" json:"created_at"`
	UpdatedAt      time.Time  `xorm:"updated 'updated_at'" json:"-"`
}

func (APIKey) TableName() string { return "api_keys" }

// APIKey is aliased so the short name stays available in this package.
type APIKey = Key

// ActiveKey reports a key that may be used: not blocked, not revoked, not expired.
func (k *Key) ActiveKey() bool {
	if k == nil || k.Status != StatusActive {
		return false
	}
	return k.ExpiresAt == nil || time.Now().Before(*k.ExpiresAt)
}

// OwnedBy reports whether the key belongs to this person.
func (k *Key) OwnedBy(userID string) bool {
	return k != nil && k.OwnerType == OwnerPersonal && k.UserID != nil && *k.UserID == userID
}

// HashKey hashes a plaintext key for storage and lookup.
func HashKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// NewPlainKey returns a new sk-... credential.
func NewPlainKey() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return "sk-" + hex.EncodeToString(b[:])
}

// KeyInput creates a key. A personal key requires UserID; a service key must
// not have one, and the database enforces the same rule.
type KeyInput struct {
	OwnerType      string
	UserID         string
	TeamID         string
	ProjectID      string
	Name           string
	Models         []string
	MaxBudget      *float64
	TPMLimit       *int
	RPMLimit       *int
	ExpiresAt      *time.Time
}

// CreateKey stores a key and returns it with the plaintext, which is shown once.
func (db *DB) CreateKey(ctx context.Context, by Actor, in KeyInput) (*Key, string, error) {
	plain := NewPlainKey()
	var out *Key
	err := db.tx(ctx, func(s *xorm.Session) error {
		if err := lockTeam(ctx, s, in.TeamID); err != nil {
			return err
		}
		team, err := getTeam(ctx, s, in.TeamID)
		if err != nil {
			return err
		}
		if team.Status != StatusActive {
			return ErrInactive
		}
		if in.OwnerType == OwnerPersonal {
			if in.UserID == "" {
				return ErrInvalid
			}
			if _, err := membership(ctx, s, in.TeamID, in.UserID); err != nil {
				return ErrInvalid
			}
		} else if in.UserID != "" {
			return ErrInvalid
		}
		if err := checkWithinTeam(s, in.TeamID, in.ProjectID, in.Models, in.MaxBudget); err != nil {
			return err
		}
		k := Key{ID: newID(), TokenHash: HashKey(plain), KeyPrefix: plain[:11], OwnerType: in.OwnerType,
			TeamID: in.TeamID, Name: in.Name, Models: nonNil(in.Models), MaxBudget: in.MaxBudget,
			TPMLimit: in.TPMLimit, RPMLimit: in.RPMLimit, Status: StatusActive, ExpiresAt: in.ExpiresAt}
		if in.UserID != "" {
			k.UserID = &in.UserID
		}
		if by.ID != "" {
			k.CreatedBy = &by.ID
		}
		if in.ProjectID != "" {
			k.ProjectID = &in.ProjectID
		}
		if _, err := s.Insert(&k); err != nil {
			return err
		}
		out = &k
		return writeAudit(s, by, Audit{Action: "key.create", ObjectType: "key", ObjectID: k.ID, TeamID: in.TeamID,
			Detail: map[string]any{"owner_type": k.OwnerType}})
	})
	if err != nil {
		return nil, "", err
	}
	return out, plain, nil
}

// RotateKey replaces the secret and returns the new plaintext once.
func (db *DB) RotateKey(ctx context.Context, by Actor, id string) (*Key, string, error) {
	plain := NewPlainKey()
	var out *Key
	err := db.tx(ctx, func(s *xorm.Session) error {
		cur, err := getKey(ctx, s, id)
		if err != nil {
			return err
		}
		if _, err := s.ID(id).Cols("token_hash", "key_prefix", "status").Update(&Key{
			TokenHash: HashKey(plain), KeyPrefix: plain[:11], Status: StatusActive}); err != nil {
			return err
		}
		if out, err = getKey(ctx, s, id); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "key.rotate", ObjectType: "key", ObjectID: id, TeamID: cur.TeamID})
	})
	if err != nil {
		return nil, "", err
	}
	return out, plain, nil
}

// DeleteKey removes a key.
func (db *DB) DeleteKey(ctx context.Context, by Actor, id string) error {
	return db.tx(ctx, func(s *xorm.Session) error {
		cur, err := getKey(ctx, s, id)
		if err != nil {
			return err
		}
		if err := affected(s.ID(id).Delete(&Key{})); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "key.delete", ObjectType: "key", ObjectID: id, TeamID: cur.TeamID})
	})
}

// SetKeyStatus blocks or unblocks a key.
func (db *DB) SetKeyStatus(ctx context.Context, by Actor, id, status string) (*Key, error) {
	var out *Key
	err := db.tx(ctx, func(s *xorm.Session) error {
		cur, err := getKey(ctx, s, id)
		if err != nil {
			return err
		}
		if _, err := s.ID(id).Cols("status").Update(&Key{Status: status}); err != nil {
			return err
		}
		if out, err = getKey(ctx, s, id); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "key.status", ObjectType: "key", ObjectID: id, TeamID: cur.TeamID,
			Detail: map[string]any{"status": status}})
	})
	return out, err
}

// UpdateKey changes the narrowing and limits of an existing key. Narrowing may
// never exceed what the team (and project) grants.
func (db *DB) UpdateKey(ctx context.Context, by Actor, id string, in KeyInput) (*Key, error) {
	var out *Key
	err := db.tx(ctx, func(s *xorm.Session) error {
		cur, err := getKey(ctx, s, id)
		if err != nil {
			return err
		}
		projectID := ""
		if cur.ProjectID != nil {
			projectID = *cur.ProjectID
		}
		if err := checkWithinTeam(s, cur.TeamID, projectID, in.Models, in.MaxBudget); err != nil {
			return err
		}
		patch := Key{Name: in.Name, Models: nonNil(in.Models), MaxBudget: in.MaxBudget,
			TPMLimit: in.TPMLimit, RPMLimit: in.RPMLimit}
		cols := []string{"name", "models", "max_budget", "tpm_limit", "rpm_limit"}
		if in.ExpiresAt != nil {
			patch.ExpiresAt, cols = in.ExpiresAt, append(cols, "expires_at")
		}
		if _, err := s.ID(id).Cols(cols...).Update(&patch); err != nil {
			return err
		}
		if out, err = getKey(ctx, s, id); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "key.update", ObjectType: "key", ObjectID: id, TeamID: cur.TeamID})
	})
	return out, err
}

// GetKey loads one key with its narrowing.
func (db *DB) GetKey(ctx context.Context, id string) (*Key, error) {
	s := db.session(ctx)
	defer s.Close()
	return getKey(ctx, s, id)
}

// KeyByHash loads a key by credential hash for authentication.
func (db *DB) KeyByHash(ctx context.Context, hash string) (*Key, error) {
	s := db.session(ctx)
	defer s.Close()
	var k Key
	ok, err := s.Where("token_hash = ?", hash).Get(&k)
	if err != nil {
		return nil, mapErr(err)
	}
	if !ok {
		return nil, ErrNotFound
	}
	return &k, nil
}

// KeyFilter scopes a key listing. Empty fields are not filtered, and a nil
// TeamIDs or KeyIDs means "no filter on that column" while an empty, non-nil
// slice means "nothing matches". The two answers stay distinct so a member with
// no teams is never shown the whole table.
type KeyFilter struct {
	TeamID    string
	ProjectID string
	UserID    string
	OwnerType string
	TeamIDs   []string
	KeyIDs    []string
	// OwnOrService narrows to UserID's personal keys together with the service
	// keys of TeamIDs. It is the team administrator's view, which no single
	// OwnerType can express because the owner rule differs per branch.
	OwnOrService bool
}

// ListKeys returns keys inside the filter, newest first.
func (db *DB) ListKeys(ctx context.Context, f KeyFilter) ([]Key, error) {
	s := db.session(ctx)
	defer s.Close()
	switch {
	case f.OwnOrService:
		if f.UserID == "" || len(f.TeamIDs) == 0 {
			return nil, nil
		}
		own := builder.Eq{"owner_type": OwnerPersonal, "user_id": f.UserID}
		service := builder.Cond(builder.Eq{"owner_type": OwnerService}).And(builder.In("team_id", f.TeamIDs))
		s = s.Where(own.Or(service))
	case f.TeamIDs != nil:
		if len(f.TeamIDs) == 0 {
			return nil, nil
		}
		s = s.In("team_id", f.TeamIDs)
	}
	if f.KeyIDs != nil {
		if len(f.KeyIDs) == 0 {
			return nil, nil
		}
		s = s.In("id", f.KeyIDs)
	}
	if f.TeamID != "" {
		s = s.Where("team_id = ?", f.TeamID)
	}
	if f.ProjectID != "" {
		s = s.Where("project_id = ?", f.ProjectID)
	}
	if f.UserID != "" && !f.OwnOrService {
		s = s.Where("user_id = ?", f.UserID)
	}
	if f.OwnerType != "" {
		s = s.Where("owner_type = ?", f.OwnerType)
	}
	var rows []Key
	if err := s.Desc("created_at").Find(&rows); err != nil {
		return nil, mapErr(err)
	}
	if len(rows) == 0 {
		return rows, nil
	}
	return rows, nil
}

// TouchKey records the last use of a key.
func (db *DB) TouchKey(ctx context.Context, id string) error {
	_, err := db.Engine.Context(ctx).ID(id).Cols("last_used_at").Update(&Key{LastUsedAt: timePtr(time.Now())})
	return err
}

func timePtr(t time.Time) *time.Time { return &t }

// RevokeTeamKeys ends every key of a team; used when a team is blocked.
func (db *DB) RevokeTeamKeys(ctx context.Context, by Actor, teamID string) error {
	return db.tx(ctx, func(s *xorm.Session) error {
		if err := lockTeam(ctx, s, teamID); err != nil {
			return err
		}
		n, err := s.Where("team_id = ? AND status <> ?", teamID, StatusRevoked).
			Cols("status").Update(&Key{Status: StatusRevoked})
		if err != nil {
			return err
		}
		logx.Info("iam revoked %d key(s) of team %s", n, teamID)
		return writeAudit(s, by, Audit{Action: "key.revoke_team", ObjectType: "team", ObjectID: teamID, TeamID: teamID})
	})
}

func getKey(ctx context.Context, s *xorm.Session, id string) (*Key, error) {
	var k Key
	if err := get(s.Where("id = ?", id), &k); err != nil {
		return nil, err
	}
	return &k, nil
}

// ---------- model resolution ----------

// allowedModels computes the permitted model set for a scope:
//
//	team    = the team's own model list
//	project = team ∩ project list (when projectID is set)
//	key     = (project or team) ∩ key list (when keyID is set)
//
// A nil result means the scope is unrestricted. An empty, non-nil result means
// it is restricted to nothing. Those are different answers and every caller
// treats them differently, so the distinction is carried out of this function
// rather than flattened into an empty slice.
//
// Each level's list narrows the one above it, and an empty list at a level
// means "inherit", not "deny": a team created a moment ago has been assigned
// nothing yet, and reading that as a denial would leave it unable to reach any
// model at all. A scope that must reach nothing is blocked instead.
//
// Every scope resolves to a single team; capabilities are never merged across
// teams. This is the only function the catalog and the inference path use.
func allowedModels(s *xorm.Session, teamID, projectID, keyID string) (map[string]struct{}, error) {
	set, err := scopeModels(s, "teams", teamID)
	if err != nil {
		return nil, err
	}
	for _, scope := range []struct{ table, id string }{
		{"projects", projectID},
		{"api_keys", keyID},
	} {
		if scope.id == "" {
			continue
		}
		narrow, err := scopeModels(s, scope.table, scope.id)
		if err != nil {
			return nil, err
		}
		// A level with no list of its own inherits the ceiling above it.
		if narrow == nil {
			continue
		}
		// The first level that does restrict becomes the ceiling; anything
		// above it was unrestricted, so there is nothing to intersect with.
		if set == nil {
			set = narrow
			continue
		}
		set = intersectModels(set, narrow)
	}
	// nil means the whole chain placed no restriction, which is a real answer
	// and not the same as "may reach nothing". Callers rely on the difference,
	// so it is passed through rather than flattened into an empty set.
	return set, nil
}

// scopeModels reads one row's model list. A nil result means the row does not
// restrict: either it does not exist or its list is empty.
func scopeModels(s *xorm.Session, table, id string) (map[string]struct{}, error) {
	if id == "" {
		return nil, nil
	}
	var row modelsRow
	ok, err := s.Table(table).Cols("models").Where("id = ?", id).Get(&row)
	if err != nil {
		return nil, mapErr(err)
	}
	if !ok || len(row.Models) == 0 {
		return nil, nil
	}
	out := make(map[string]struct{}, len(row.Models))
	for _, m := range row.Models {
		if m != "" {
			out[m] = struct{}{}
		}
	}
	return out, nil
}

// intersectModels returns the models present in both sets.
func intersectModels(a, b map[string]struct{}) map[string]struct{} {
	out := map[string]struct{}{}
	for m := range a {
		if _, ok := b[m]; ok {
			out[m] = struct{}{}
		}
	}
	return out
}

// modelsRow decodes a TEXT column holding a JSON array. Projecting the column
// straight into []string yields the raw document as a single element.
type modelsRow struct {
	Models []string `xorm:"json 'models'"`
}

// AllowedModelsForTeam is the team's model set, used before a key exists.
//
// A nil result means the team has no restriction of its own.
func (db *DB) AllowedModelsForTeam(ctx context.Context, teamID string) ([]string, error) {
	s := db.session(ctx)
	defer s.Close()
	set, err := allowedModels(s, teamID, "", "")
	if err != nil {
		return nil, err
	}
	return sortedKeys(set), nil
}

// AllowedModelsForKey is the key's effective model set; the catalog and the
// inference path both call it so a listed model is always a usable model.
func (db *DB) AllowedModelsForKey(ctx context.Context, k *Key) ([]string, error) {
	s := db.session(ctx)
	defer s.Close()
	projectID := ""
	if k.ProjectID != nil {
		projectID = *k.ProjectID
	}
	set, err := allowedModels(s, k.TeamID, projectID, k.ID)
	if err != nil {
		return nil, err
	}
	return sortedKeys(set), nil
}

// budgetCeiling is the tightest budget above a scope: the team's, the
// project's, and the key's.
func budgetCeiling(s *xorm.Session, teamID, projectID, keyID string) (*float64, error) {
	var team Team
	if err := get(s.Where("id = ?", teamID), &team); err != nil {
		return nil, err
	}
	ceiling := team.MaxBudget
	if projectID != "" {
		var p Project
		if err := get(s.Where("id = ?", projectID), &p); err != nil {
			return nil, err
		}
		if p.MaxBudget != nil && (ceiling == nil || *p.MaxBudget < *ceiling) {
			ceiling = p.MaxBudget
		}
	}
	if keyID != "" {
		var k Key
		if err := get(s.Where("id = ?", keyID), &k); err != nil {
			return nil, err
		}
		if k.MaxBudget != nil && (ceiling == nil || *k.MaxBudget < *ceiling) {
			ceiling = k.MaxBudget
		}
	}
	return ceiling, nil
}

// sortedKeys renders a model set as a sorted list. A nil set stays nil, because
// nil means "unrestricted" and an empty slice means "may reach nothing"; those
// are different answers and the callers branch on them.
func sortedKeys(set map[string]struct{}) []string {
	if set == nil {
		return nil
	}
	out := make([]string, 0, len(set))
	for m := range set {
		out = append(out, m)
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// KeyNameFromPlain derives a display name from a credential.
func KeyNameFromPlain(plain string) string {
	if len(plain) > 7 {
		return plain[:7] + "…"
	}
	return strings.TrimSpace(plain)
}

// ParseExpiry turns a relative duration such as 30s, 30m, 30h or 30d into the
// absolute instant it names, counted from now. An empty value is "no expiry"
// and returns nil. Anything else is an error, so a typo never becomes a key
// that never expires.
func ParseExpiry(d string) (*time.Time, error) {
	d = strings.TrimSpace(d)
	if d == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(d[:len(d)-1])
	if err != nil {
		return nil, fmt.Errorf("invalid duration %q", d)
	}
	var at time.Time
	switch d[len(d)-1] {
	case 's':
		at = time.Now().Add(time.Duration(n) * time.Second)
	case 'm':
		at = time.Now().Add(time.Duration(n) * time.Minute)
	case 'h':
		at = time.Now().Add(time.Duration(n) * time.Hour)
	case 'd':
		at = time.Now().AddDate(0, 0, n)
	default:
		return nil, fmt.Errorf("invalid duration %q", d)
	}
	at = at.UTC()
	return &at, nil
}

// ResetKeySpend sets a key's spend back to zero, or to the given value. The
// usage events behind the old figure stay, so history is never rewritten.
func (db *DB) ResetKeySpend(ctx context.Context, by Actor, id string, to float64) (*Key, error) {
	var out *Key
	err := db.tx(ctx, func(s *xorm.Session) error {
		cur, err := getKey(ctx, s, id)
		if err != nil {
			return err
		}
		if _, err := s.ID(id).Cols("spend").Update(&Key{Spend: to}); err != nil {
			return err
		}
		if out, err = getKey(ctx, s, id); err != nil {
			return err
		}
		return writeAudit(s, by, Audit{Action: "key.reset_spend", ObjectType: "key", ObjectID: id, TeamID: cur.TeamID,
			Detail: map[string]any{"to": to}})
	})
	return out, err
}
