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
	MaterializeDB
	PrepareStaging() error
	TruncateReferences(context.Context) error
	ComputeDelta(context.Context) (rebuild, deletes []string, err error)
	ApplyDelta(context.Context) error
	DeleteDocuments(context.Context, []string) error
}

// Update applies a new monthly snapshot incrementally: it loads the snapshot
// into the staging tables, diffs them against the live tables, applies only the
// changes, and rebuilds only the affected documents. Source ZIPs must already be
// in dir, as with Transform.
func Update(dir string, db incrementalDB, batch, maxDB int, privacy bool) error {
	ibgeMunicipalitiesURL, err := ibgeMunicipalitiesURL()
	if err != nil {
		return fmt.Errorf("could not discover ibge municipalities URL: %w", err)
	}
	return update(dir, db, batch, maxDB, privacy, ibgeMunicipalitiesURL)
}

func update(dir string, db incrementalDB, batch, maxDB int, privacy bool, ibgeMunicipalitiesURL string) error {
	ctx := context.Background()
	if err := db.PrepareStaging(); err != nil {
		return fmt.Errorf("could not prepare staging tables: %w", err)
	}
	if err := db.TruncateReferences(ctx); err != nil {
		return fmt.Errorf("could not reset reference tables: %w", err)
	}
	slog.Info("Loading the new snapshot into staging…")
	label, err := loadSnapshot(dir, db, batch, maxDB, privacy, ibgeMunicipalitiesURL, "_staging", false)
	if err != nil {
		return err
	}
	slog.Info("Computing the delta…")
	rebuild, deletes, err := db.ComputeDelta(ctx)
	if err != nil {
		return err
	}
	slog.Info("Delta computed", "rebuild", len(rebuild), "delete", len(deletes))
	if err := db.ApplyDelta(ctx); err != nil {
		return fmt.Errorf("could not apply the delta: %w", err)
	}
	slog.Info("Materializing the affected documents…")
	if err := Materialize(ctx, db, rebuild, privacy, slog.Default()); err != nil {
		return fmt.Errorf("could not materialize the rebuilt documents: %w", err)
	}
	if err := db.DeleteDocuments(ctx, deletes); err != nil {
		return fmt.Errorf("could not delete removed documents: %w", err)
	}
	return saveUpdatedAt(db, label)
}
