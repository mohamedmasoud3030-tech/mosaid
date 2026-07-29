package cognitive

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteGoalStore persists goals and runs to SQLite for crash resume.
type SQLiteGoalStore struct {
	db *sql.DB
}

// NewSQLiteGoalStore opens or creates a SQLite-backed goal store.
func NewSQLiteGoalStore(dbPath string) (*SQLiteGoalStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, err
	}
	if err = migrateGoalStore(db); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLiteGoalStore{db: db}, nil
}

func migrateGoalStore(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS cognitive_goals(
	id TEXT PRIMARY KEY,
	owner_id TEXT NOT NULL,
	source_message TEXT NOT NULL,
	state TEXT NOT NULL,
	constraints_json BLOB NOT NULL DEFAULT '{}',
	success_criteria_json BLOB NOT NULL DEFAULT '[]',
	risks_json BLOB NOT NULL DEFAULT '[]',
	pending_inputs_json BLOB NOT NULL DEFAULT '[]',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS cognitive_runs(
	id TEXT PRIMARY KEY,
	goal_id TEXT NOT NULL,
	state TEXT NOT NULL,
	current_step_idx INTEGER NOT NULL DEFAULT 0,
	steps_json BLOB NOT NULL DEFAULT '[]',
	budgets_json BLOB NOT NULL DEFAULT '{}',
	budget_used_json BLOB NOT NULL DEFAULT '{}',
	evidence_json BLOB NOT NULL DEFAULT '[]',
	audit_refs_json BLOB NOT NULL DEFAULT '[]',
	replan_count INTEGER NOT NULL DEFAULT 0,
	max_replans INTEGER NOT NULL DEFAULT 3,
	error TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	resumable_at TEXT,
	FOREIGN KEY(goal_id) REFERENCES cognitive_goals(id)
);
CREATE INDEX IF NOT EXISTS cognitive_runs_goal_idx ON cognitive_runs(goal_id);
CREATE INDEX IF NOT EXISTS cognitive_runs_state_idx ON cognitive_runs(state);
`)
	return err
}

func (s *SQLiteGoalStore) Close() error { return s.db.Close() }

func (s *SQLiteGoalStore) SaveGoal(ctx context.Context, goal Goal) error {
	constraints, _ := json.Marshal(goal.Constraints)
	criteria, _ := json.Marshal(goal.SuccessCriteria)
	risks, _ := json.Marshal(goal.Risks)
	pending, _ := json.Marshal(goal.PendingInputs)

	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO cognitive_goals(id,owner_id,source_message,state,constraints_json,success_criteria_json,risks_json,pending_inputs_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		goal.ID, goal.OwnerID, goal.SourceMessage, string(goal.State),
		constraints, criteria, risks, pending,
		goal.CreatedAt.Format(time.RFC3339Nano),
		goal.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteGoalStore) GetGoal(ctx context.Context, id string) (Goal, error) {
	var goal Goal
	var state, createdAt, updatedAt string
	var constraints, criteria, risks, pending []byte

	err := s.db.QueryRowContext(ctx,
		`SELECT id,owner_id,source_message,state,constraints_json,success_criteria_json,risks_json,pending_inputs_json,created_at,updated_at FROM cognitive_goals WHERE id=?`, id,
	).Scan(&goal.ID, &goal.OwnerID, &goal.SourceMessage, &state,
		&constraints, &criteria, &risks, &pending,
		&createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Goal{}, fmt.Errorf("%w: %s", ErrGoalNotFound, id)
	}
	if err != nil {
		return Goal{}, err
	}

	goal.State = GoalState(state)
	json.Unmarshal(constraints, &goal.Constraints)
	json.Unmarshal(criteria, &goal.SuccessCriteria)
	json.Unmarshal(risks, &goal.Risks)
	json.Unmarshal(pending, &goal.PendingInputs)
	goal.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	goal.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return goal, nil
}

func (s *SQLiteGoalStore) SaveRun(ctx context.Context, run Run) error {
	steps, _ := json.Marshal(run.Steps)
	budgets, _ := json.Marshal(run.Budgets)
	budgetUsed, _ := json.Marshal(run.BudgetUsed)
	evidence, _ := json.Marshal(run.Evidence)
	auditRefs, _ := json.Marshal(run.AuditRefs)

	resumable := ""
	if !run.ResumableAt.IsZero() {
		resumable = run.ResumableAt.Format(time.RFC3339Nano)
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO cognitive_runs(id,goal_id,state,current_step_idx,steps_json,budgets_json,budget_used_json,evidence_json,audit_refs_json,replan_count,max_replans,error,created_at,updated_at,resumable_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		run.ID, run.GoalID, string(run.State), run.CurrentStepIdx,
		steps, budgets, budgetUsed, evidence, auditRefs,
		run.ReplanCount, run.MaxReplans, run.Error,
		run.CreatedAt.Format(time.RFC3339Nano),
		run.UpdatedAt.Format(time.RFC3339Nano),
		resumable,
	)
	return err
}

func (s *SQLiteGoalStore) GetRun(ctx context.Context, id string) (Run, error) {
	var run Run
	var state, createdAt, updatedAt, resumable string
	var steps, budgets, budgetUsed, evidence, auditRefs []byte

	err := s.db.QueryRowContext(ctx,
		`SELECT id,goal_id,state,current_step_idx,steps_json,budgets_json,budget_used_json,evidence_json,audit_refs_json,replan_count,max_replans,error,created_at,updated_at,resumable_at FROM cognitive_runs WHERE id=?`, id,
	).Scan(&run.ID, &run.GoalID, &state, &run.CurrentStepIdx,
		&steps, &budgets, &budgetUsed, &evidence, &auditRefs,
		&run.ReplanCount, &run.MaxReplans, &run.Error,
		&createdAt, &updatedAt, &resumable,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, fmt.Errorf("%w: %s", ErrRunNotFound, id)
	}
	if err != nil {
		return Run{}, err
	}

	run.State = RunState(state)
	json.Unmarshal(steps, &run.Steps)
	json.Unmarshal(budgets, &run.Budgets)
	json.Unmarshal(budgetUsed, &run.BudgetUsed)
	json.Unmarshal(evidence, &run.Evidence)
	json.Unmarshal(auditRefs, &run.AuditRefs)
	run.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	run.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if resumable != "" {
		run.ResumableAt, _ = time.Parse(time.RFC3339Nano, resumable)
	}
	return run, nil
}

func (s *SQLiteGoalStore) ListRuns(ctx context.Context, goalID string) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,goal_id,state,current_step_idx,steps_json,budgets_json,budget_used_json,evidence_json,audit_refs_json,replan_count,max_replans,error,created_at,updated_at,resumable_at FROM cognitive_runs WHERE goal_id=? ORDER BY created_at`, goalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		var run Run
		var state, createdAt, updatedAt, resumable string
		var steps, budgets, budgetUsed, evidence, auditRefs []byte

		if err := rows.Scan(&run.ID, &run.GoalID, &state, &run.CurrentStepIdx,
			&steps, &budgets, &budgetUsed, &evidence, &auditRefs,
			&run.ReplanCount, &run.MaxReplans, &run.Error,
			&createdAt, &updatedAt, &resumable,
		); err != nil {
			return nil, err
		}

		run.State = RunState(state)
		json.Unmarshal(steps, &run.Steps)
		json.Unmarshal(budgets, &run.Budgets)
		json.Unmarshal(budgetUsed, &run.BudgetUsed)
		json.Unmarshal(evidence, &run.Evidence)
		json.Unmarshal(auditRefs, &run.AuditRefs)
		run.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		run.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		if resumable != "" {
			run.ResumableAt, _ = time.Parse(time.RFC3339Nano, resumable)
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}
