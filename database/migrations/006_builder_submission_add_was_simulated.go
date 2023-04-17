package migrations

import (
	"github.com/flashbots/mev-boost-relay/database/vars"
	migrate "github.com/rubenv/sql-migrate"
)

var Migration006BuilderSubmissionWasSimulated = &migrate.Migration{
	Id: "006-builder-submission-was-simulated",
	Up: []string{`
		ALTER TABLE ` + vars.TableBuilderBlockSubmission + ` ADD was_simulated boolean NOT NULL DEFAULT true;
	`},
	Down: []string{},

	DisableTransactionUp:   true,
	DisableTransactionDown: true,
}
