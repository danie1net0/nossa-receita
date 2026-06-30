package cmd

import (
	"fmt"

	"codeberg.org/cuducos/minha-receita/transform"
	"github.com/spf13/cobra"
)

const updateHelper = `
Applies a new monthly snapshot incrementally.

Loads the snapshot into staging tables, diffs them against the live data,
applies only the changes (inserts, updates and deletes), and rebuilds only the
affected JSON documents — instead of recreating the whole database.

Source ZIPs must be placed in the data directory, as with the transform command.`

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Applies a new snapshot incrementally",
	Long:  updateHelper,
	RunE: func(_ *cobra.Command, _ []string) error {
		pg, err := loadPostgres(&args)
		if err != nil {
			return fmt.Errorf("could not find database: %w", err)
		}
		defer pg.Close()
		return transform.Update(dir, pg, batchSize, maxParallelDBQueries, !noPrivacy)
	},
}

func updateCLI() *cobra.Command {
	updateCmd.Flags().IntVarP(&maxParallelDBQueries, "max-parallel-db-queries", "m", transform.MaxParallelDBQueries, "maximum parallel database queries")
	updateCmd.Flags().IntVarP(&batchSize, "batch-size", "b", transform.PostgresBatchSize, "size of the batch to save to the database")
	updateCmd.Flags().BoolVarP(&noPrivacy, "no-privacy", "p", noPrivacy, "include email addresses, CPF and other PII in the JSON data")
	addDatabase(updateCmd, &args)
	return updateCmd
}
