package planstore

import (
	"context"

	"github.com/contenox/contenox/libdbexec"
)

// InitSchema creates the plans and plan_steps tables if they do not exist.
func InitSchema(ctx context.Context, exec libdbexec.Exec) error {
	_, err := exec.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS plans (
			id         VARCHAR(255) PRIMARY KEY,
			name       VARCHAR(255) NOT NULL UNIQUE,
			goal       TEXT         NOT NULL,
			status     VARCHAR(50)  NOT NULL DEFAULT 'active',
			session_id VARCHAR(255),
			created_at TIMESTAMP    NOT NULL,
			updated_at TIMESTAMP    NOT NULL
		);

		CREATE TABLE IF NOT EXISTS plan_steps (
			id               VARCHAR(255) PRIMARY KEY,
			plan_id          VARCHAR(255) NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
			ordinal          INT          NOT NULL,
			description      TEXT         NOT NULL,
			status           VARCHAR(50)  NOT NULL DEFAULT 'pending',
			execution_result TEXT         NOT NULL DEFAULT '',
			executed_at      TIMESTAMP,
			UNIQUE (plan_id, ordinal)
		);

		CREATE INDEX IF NOT EXISTS idx_plan_steps_plan_id ON plan_steps(plan_id);
	`)
	return err
}
