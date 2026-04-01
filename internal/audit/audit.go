package audit

import (
	"context"
	"encoding/json"
	"net"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ActorType string

const (
	ActorTypeUser    ActorType = "USER"
	ActorTypeAgent   ActorType = "AGENT"
	ActorTypeAPIKey  ActorType = "API_KEY"
)

type AuditLogger struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *AuditLogger {
	return &AuditLogger{pool: pool}
}

type Entry struct {
	OrgID        string
	ActorType    ActorType
	ActorID      string
	ActorEmail   *string
	Action       string
	ResourceType *string
	ResourceID   *string
	Metadata     map[string]any
	IP           *string
	UserAgent    *string
}

func (l *AuditLogger) Log(ctx context.Context, e Entry) error {
	metadataJSON, err := json.Marshal(e.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	_, err = l.pool.Exec(ctx, `
		INSERT INTO audit_log (org_id, actor_type, actor_id, actor_email, action, resource_type, resource_id, metadata, ip, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, e.OrgID, e.ActorType, e.ActorID, e.ActorEmail, e.Action, e.ResourceType, e.ResourceID, metadataJSON, e.IP, e.UserAgent)

	return err
}

func GetIP(r *http.Request) string {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ip = xff
	}
	return ip
}
