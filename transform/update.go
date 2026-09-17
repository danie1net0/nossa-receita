package transform

import (
	"context"
	"fmt"
	"log/slog"
)

// incrementalDB is the database surface needed to apply a snapshot incrementally.
// It is implemented by the PostgreSQL backend.
type incrementalDB interface {
	database
	PrepareStaging() error
	ApplyDelta(context.Context) error
}

// Update applies a new monthly snapshot incrementally: it loads the snapshot
// into the staging tables and changes only the live rows that differ from it.
// Source ZIPs must already be in dir, as with Transform.
func Update(dir string, db incrementalDB, batch int) error {
	ibgeMunicipalitiesURL, err := ibgeMunicipalitiesURL()
	if err != nil {
		return fmt.Errorf("could not discover ibge municipalities URL: %w", err)
	}
	return update(dir, db, batch, ibgeMunicipalitiesURL)
}

func update(dir string, db incrementalDB, batch int, ibgeMunicipalitiesURL string) error {
	ctx := context.Background()
	if err := db.PrepareStaging(); err != nil {
		return fmt.Errorf("could not prepare staging tables: %w", err)
	}
	slog.Info("Loading the new snapshot into staging…")
	label, err := loadSnapshot(dir, db, batch, ibgeMunicipalitiesURL, "_staging")
	if err != nil {
		return err
	}
	slog.Info("Applying the changes…")
	if err := db.ApplyDelta(ctx); err != nil {
		return fmt.Errorf("could not apply the delta: %w", err)
	}
	return saveUpdatedAt(db, label)
}
