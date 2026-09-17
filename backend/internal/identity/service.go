package identity

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/idgen"
)

type User struct {
	ID          string `json:"id"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name"`
}

type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type MembershipView struct {
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	Role             string `json:"role"`
}

type Service struct {
	pool *db.Pool
}

func NewService(pool *db.Pool) *Service {
	return &Service{pool: pool}
}

// ResolveUser upserts a user by (issuer, subject) and returns their current
// record. This runs before any organization is known, so it is one of the
// few legitimately unscoped operations (see db.Pool.GlobalTx).
func (s *Service) ResolveUser(ctx context.Context, claims auth.Claims) (User, error) {
	var u User
	err := s.pool.GlobalTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		displayName := claims.Name
		if displayName == "" {
			displayName = claims.Email
		}
		if displayName == "" {
			displayName = claims.Subject
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO users (id, oidc_issuer, oidc_subject, email, display_name)
			VALUES ($1, $2, $3, NULLIF($4, ''), $5)
			ON CONFLICT (oidc_issuer, oidc_subject) DO UPDATE SET
				email = COALESCE(NULLIF(EXCLUDED.email, ''), users.email),
				display_name = EXCLUDED.display_name
			RETURNING id, coalesce(email, ''), display_name`,
			idgen.New("user"), claims.Issuer, claims.Subject, claims.Email, displayName)
		return row.Scan(&u.ID, &u.Email, &u.DisplayName)
	})
	if err != nil {
		return User{}, fmt.Errorf("resolve user: %w", err)
	}
	return u, nil
}

// Memberships lists every organization userID belongs to, across
// organizations, using the self-visibility RLS policy (migration 00008)
// rather than any single organization scope.
func (s *Service) Memberships(ctx context.Context, userID string) ([]MembershipView, error) {
	var out []MembershipView
	err := s.pool.SelfTx(ctx, userID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT m.organization_id, o.name, m.role
			FROM memberships m JOIN organizations o ON o.id = m.organization_id
			WHERE m.user_id = $1 AND m.status = 'active'
			ORDER BY o.name`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var mv MembershipView
			if err := rows.Scan(&mv.OrganizationID, &mv.OrganizationName, &mv.Role); err != nil {
				return err
			}
			out = append(out, mv)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("list memberships: %w", err)
	}
	return out, nil
}

// ActiveMembership returns userID's role in organizationID, or ok=false if
// they have no active membership there. Callers use this to resolve the
// Actor attached to each request.
func (s *Service) ActiveMembership(ctx context.Context, organizationID, userID string) (role string, ok bool, err error) {
	txErr := s.pool.Tx(ctx, organizationID, userID, func(ctx context.Context, tx pgx.Tx) error {
		scanErr := tx.QueryRow(ctx, `
			SELECT role FROM memberships
			WHERE organization_id = $1 AND user_id = $2 AND status = 'active'`,
			organizationID, userID).Scan(&role)
		if scanErr == pgx.ErrNoRows {
			return nil
		}
		if scanErr == nil {
			ok = true
		}
		return scanErr
	})
	if txErr != nil {
		return "", false, fmt.Errorf("check membership: %w", txErr)
	}
	return role, ok, nil
}

// CreateOrganizationWithOwner creates a new organization and makes ownerID
// its first owner, atomically. Nothing gates who may call this: any
// authenticated user may start an organization, the same way creating a new
// workspace works on comparable platforms. It is unscoped (db.Pool.GlobalTx)
// because the organization does not exist yet to scope to.
func (s *Service) CreateOrganizationWithOwner(ctx context.Context, ownerID, name string) (Organization, error) {
	org := Organization{ID: idgen.New("organization"), Name: name}
	err := s.pool.GlobalTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, org.ID, org.Name); err != nil {
			return err
		}
		// The membership row is tenant-scoped and Row Level Security applies
		// to forge_app regardless of transaction history; scope this
		// transaction to the organization we just created before inserting
		// its first row.
		if err := s.pool.SetScope(ctx, tx, org.ID, ownerID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO memberships (id, organization_id, user_id, role, status)
			VALUES ($1, $2, $3, 'owner', 'active')`,
			idgen.New("membership"), org.ID, ownerID)
		return err
	})
	if err != nil {
		return Organization{}, fmt.Errorf("create organization: %w", err)
	}
	return org, nil
}
