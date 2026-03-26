package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/XferOps/hizal/internal/mcp"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ReviewHandlers struct {
	pool *pgxpool.Pool
}

func NewReviewHandlers(pool *pgxpool.Pool) *ReviewHandlers {
	return &ReviewHandlers{pool: pool}
}

type ReviewInboxItem struct {
	ID                string            `json:"id"`
	QueryKey          string            `json:"query_key"`
	Title             string            `json:"title"`
	Content           string            `json:"content"`
	Scope             string            `json:"scope"`
	ChunkType         string            `json:"chunk_type"`
	ProjectID         *string           `json:"project_id"`
	ProjectName       *string           `json:"project_name"`
	LastReviewAt      *time.Time        `json:"last_review_at"`
	LastActivityAt    time.Time         `json:"last_activity_at"`
	DaysSinceReview   *int              `json:"days_since_review,omitempty"`
	DaysSinceActivity int               `json:"days_since_activity"`
	StaleSignals      []StaleSignal     `json:"stale_signals"`
	MinUsefulness     *int              `json:"min_usefulness"`
	MinCorrectness    *int              `json:"min_correctness"`
	AvgUsefulness     *float64          `json:"avg_usefulness,omitempty"`
	AvgCorrectness    *float64          `json:"avg_correctness,omitempty"`
	ReviewCount       int               `json:"review_count"`
	Freshness         float64           `json:"freshness"`
	Reason            ReviewInboxReason `json:"reason"`
}

type ReviewInboxReason struct {
	Category       string     `json:"category"`
	SignalStrength string     `json:"signal_strength"`
	Summary        string     `json:"summary"`
	Action         string     `json:"action,omitempty"`
	Note           string     `json:"note,omitempty"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty"`
}

type StaleSignal struct {
	Action string `json:"action"`
	Note   string `json:"note,omitempty"`
}

type ReviewInboxResponse struct {
	Chunks          []ReviewInboxItem `json:"chunks"`
	Total           int               `json:"total"`
	ActionableTotal int               `json:"actionable_total"`
}

func signalStrengthForCategory(category string) string {
	switch category {
	case "agent_flag":
		return "high"
	case "low_score":
		return "medium"
	default:
		return "low"
	}
}

func summaryForReason(category, action, note string, avgUsefulness, avgCorrectness *float64, reviewCount, daysSinceActivity int) string {
	switch category {
	case "agent_flag":
		summary := "Flagged for review"
		if action != "" {
			summary = "Flagged as " + action
		}
		if note != "" {
			return summary + ": " + note
		}
		return summary
	case "low_score":
		parts := "Low review scores"
		if avgUsefulness != nil && avgCorrectness != nil {
			parts = "Low review scores (usefulness " + formatScore(*avgUsefulness) + "/5, correctness " + formatScore(*avgCorrectness) + "/5)"
		}
		if reviewCount > 0 {
			return parts + " across " + pluralizeReviews(reviewCount)
		}
		return parts
	default:
		return "No activity in " + pluralizeDays(daysSinceActivity)
	}
}

func formatScore(score float64) string {
	return strconv.FormatFloat(score, 'f', 1, 64)
}

func pluralizeReviews(n int) string {
	if n == 1 {
		return "1 review"
	}
	return strconv.Itoa(n) + " reviews"
}

func pluralizeDays(n int) string {
	if n == 1 {
		return "1 day"
	}
	return strconv.Itoa(n) + " days"
}

func (h *ReviewHandlers) ReviewInbox(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	// Require auth — either API key claims or JWT user.
	// In production the JWT-protected route group handles this via middleware,
	// but we guard explicitly for defense-in-depth and direct-handler tests.
	if _, ok := ClaimsFrom(r.Context()); !ok {
		if _, ok := JWTUserFrom(r.Context()); !ok {
			writeError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required")
			return
		}
	}
	ctx := r.Context()

	rows, err := h.pool.Query(ctx, `
		WITH org_projects AS (
			SELECT id FROM projects WHERE org_id = $1
		),
		org_chunks AS (
			SELECT id, project_id, org_id, query_key, title, content, scope, chunk_type, updated_at
			FROM context_chunks
			WHERE org_id = $1 OR project_id IN (SELECT id FROM org_projects)
		),
			review_stats AS (
			SELECT
				chunk_id,
				MAX(created_at) AS last_review_at,
				COUNT(*) AS review_count,
				AVG(usefulness::float) FILTER (WHERE usefulness IS NOT NULL) AS avg_usefulness,
				AVG(correctness::float) FILTER (WHERE correctness IS NOT NULL) AS avg_correctness,
				MIN(usefulness) AS min_usefulness,
				MIN(correctness) AS min_correctness
			FROM context_reviews
			WHERE chunk_id IN (SELECT id FROM org_chunks)
			GROUP BY chunk_id
		),
		latest_review AS (
			SELECT DISTINCT ON (chunk_id)
				chunk_id,
				action,
				COALESCE(NULLIF(correctness_note, ''), NULLIF(usefulness_note, ''), NULLIF(task, '')) AS note,
				created_at
			FROM context_reviews
			WHERE chunk_id IN (SELECT id FROM org_chunks)
			ORDER BY chunk_id, created_at DESC
		),
		needs_review AS (
			SELECT
				oc.id,
				oc.query_key,
				oc.title,
				oc.content,
				oc.scope,
				oc.chunk_type,
				oc.project_id,
				rs.last_review_at,
				COALESCE(rs.last_review_at, oc.updated_at) AS last_activity,
				COALESCE(rs.review_count, 0) AS review_count,
				rs.avg_usefulness,
				rs.avg_correctness,
				rs.min_usefulness,
				rs.min_correctness,
				CASE
					WHEN lr.action IN ('needs_update', 'outdated', 'incorrect') THEN 'agent_flag'
					WHEN COALESCE(rs.avg_usefulness, 5) < 3 OR COALESCE(rs.avg_correctness, 5) < 3 THEN 'low_score'
					WHEN COALESCE(rs.last_review_at, oc.updated_at) < NOW() - INTERVAL '60 days' THEN 'aging'
					ELSE NULL
				END AS reason_category,
				lr.action AS latest_action,
				lr.note AS latest_note,
				lr.created_at AS latest_review_created_at
			FROM org_chunks oc
			LEFT JOIN review_stats rs ON rs.chunk_id = oc.id
			LEFT JOIN latest_review lr ON lr.chunk_id = oc.id
		WHERE
			COALESCE(lr.action, '') != 'dismiss_flag'
			AND (
				lr.action IN ('needs_update', 'outdated', 'incorrect')
				OR COALESCE(rs.avg_usefulness, 5) < 3
				OR COALESCE(rs.avg_correctness, 5) < 3
				OR COALESCE(rs.last_review_at, oc.updated_at) < NOW() - INTERVAL '60 days'
			)
		)
		SELECT
			nr.id,
			nr.query_key,
			nr.title,
			nr.content,
			nr.scope,
			nr.chunk_type,
			nr.project_id,
			p.name AS project_name,
			nr.last_review_at,
			nr.last_activity,
			nr.review_count,
			nr.avg_usefulness,
			nr.avg_correctness,
			nr.min_usefulness,
			nr.min_correctness,
			nr.reason_category,
			nr.latest_action,
			nr.latest_note,
			nr.latest_review_created_at,
			ARRAY(
				SELECT jsonb_build_object(
					'action', CASE
						WHEN cr.action IN ('needs_update', 'outdated', 'incorrect') THEN cr.action
						ELSE 'low_score'
					END,
					'note', COALESCE(
						NULLIF(cr.usefulness_note, ''),
						NULLIF(cr.correctness_note, '')
					)
				)
				FROM context_reviews cr
				WHERE cr.chunk_id = nr.id
				AND (
					cr.action IN ('needs_update', 'outdated', 'incorrect')
					OR cr.usefulness < 3
					OR cr.correctness < 3
				)
				ORDER BY cr.created_at DESC
			) AS stale_signals_json
		FROM needs_review nr
		LEFT JOIN projects p ON p.id = nr.project_id
		ORDER BY CASE nr.reason_category
			WHEN 'agent_flag' THEN 0
			WHEN 'low_score' THEN 1
			ELSE 2
		END ASC, nr.last_activity ASC
	`, orgID)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}
	defer rows.Close()

	var items []ReviewInboxItem

	for rows.Next() {
		var (
			id, queryKey, title, content, scope, chunkType string
			projectID                             *string
			projectName                           *string
			lastReviewAt                          *time.Time
			lastActivity                          time.Time
			reviewCount                           int
			avgUsefulness, avgCorrectness         *float64
			minUsefulness, minCorrectness         *int
			reasonCategory                        string
			latestAction                          *string
			latestNote                            *string
			latestReviewCreatedAt                 *time.Time
			signalsJSON                           [][]byte
		)
		if err := rows.Scan(&id, &queryKey, &title, &content, &scope, &chunkType, &projectID, &projectName, &lastReviewAt, &lastActivity, &reviewCount, &avgUsefulness, &avgCorrectness, &minUsefulness, &minCorrectness, &reasonCategory, &latestAction, &latestNote, &latestReviewCreatedAt, &signalsJSON); err != nil {
			writeInternalError(r, w, "DB_ERROR", err)
			return
		}

		signals := make([]StaleSignal, 0)
		for _, b := range signalsJSON {
			if len(b) == 0 {
				continue
			}
			var s struct {
				Action string `json:"action"`
				Note   string `json:"note"`
			}
			if json.Unmarshal(b, &s) == nil {
				signals = append(signals, StaleSignal{Action: s.Action, Note: s.Note})
			}
		}

		daysSinceActivity := int(time.Since(lastActivity).Hours() / 24)
		var daysSinceReview *int
		if lastReviewAt != nil {
			d := int(time.Since(*lastReviewAt).Hours() / 24)
			daysSinceReview = &d
		}
		freshness := 1.0
		if daysSinceActivity > int(mcp.FreshnessDecayStartDays) {
			decay := float64(daysSinceActivity-int(mcp.FreshnessDecayStartDays)) / (mcp.FreshnessDecayFullDays - mcp.FreshnessDecayStartDays)
			if decay >= 1.0 {
				freshness = mcp.FreshnessMin
			} else {
				freshness = 1.0 - (1.0-mcp.FreshnessMin)*decay
			}
		}

		reason := ReviewInboxReason{
			Category:       reasonCategory,
			SignalStrength: signalStrengthForCategory(reasonCategory),
			ReviewedAt:     latestReviewCreatedAt,
		}
		if latestAction != nil {
			reason.Action = *latestAction
		}
		if latestNote != nil {
			reason.Note = *latestNote
		}
		reason.Summary = summaryForReason(reasonCategory, reason.Action, reason.Note, avgUsefulness, avgCorrectness, reviewCount, daysSinceActivity)

		items = append(items, ReviewInboxItem{
			ID:                id,
			QueryKey:          queryKey,
			Title:             title,
			Content:           content,
			Scope:             scope,
			ChunkType:         chunkType,
			ProjectID:         projectID,
			ProjectName:       projectName,
			LastReviewAt:      lastReviewAt,
			LastActivityAt:    lastActivity,
			DaysSinceReview:   daysSinceReview,
			DaysSinceActivity: daysSinceActivity,
			StaleSignals:      signals,
			MinUsefulness:     minUsefulness,
			MinCorrectness:    minCorrectness,
			AvgUsefulness:     avgUsefulness,
			AvgCorrectness:    avgCorrectness,
			ReviewCount:       reviewCount,
			Freshness:         freshness,
			Reason:            reason,
		})
	}

	if err := rows.Err(); err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}

	if items == nil {
		items = []ReviewInboxItem{}
	}

	actionableTotal := 0
	for _, item := range items {
		if item.Reason.Category != "aging" {
			actionableTotal++
		}
	}

	writeJSON(w, http.StatusOK, ReviewInboxResponse{
		Chunks:          items,
		Total:           len(items),
		ActionableTotal: actionableTotal,
	})
}
