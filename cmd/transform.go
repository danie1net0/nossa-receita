package cmd

import (
	"fmt"

	"codeberg.org/cuducos/minha-receita/transform"
	"github.com/spf13/cobra"
)

const transformHelper = `
Loads the CSV files from the Federal Revenue into the relational tables of the
database (empresa, estabelecimento, socio, simples, regime_tributario,
estabelecimento_cnae and the reference tables).

The transformation process is divided into three steps:
1. Unarchive the bundled ZIP file for a given month/year
2. Load the source files to a key-value store
3. Load the database from the key-value store and the establishment files
`

var (
	batchSize int
	cleanUp   bool
)

var transformCmd = &cobra.Command{
	Use:   "transform",
	Short: "Transforms the CSV files into database records",
	Long:  transformHelper,
	RunE: func(_ *cobra.Command, _ []string) error {
		db, err := loadDatabase(&args)
		if err != nil {
			return fmt.Errorf("could not find database: %w", err)
		}
		defer db.Close()
		if cleanUp {
			if err := db.Drop(); err != nil {
				return err
			}
			if err := db.Create(); err != nil {
				return err
			}
		}
		return transform.Transform(dir, db, batchSize)
	},
}

var cleanupTempCmd = &cobra.Command{
	Use:   "clean-up",
	Short: "Clean-up temporary ETL files",
	RunE: func(_ *cobra.Command, _ []string) error {
		return transform.Cleanup()
	},
}

func transformCLI() *cobra.Command {
	batchSize = transform.PostgresBatchSize

	transformCmd.Flags().BoolVarP(&cleanUp, "clean-up", "c", cleanUp, "drop & recreate the database tables before starting")
	transformCmd.Flags().IntVarP(&batchSize, "batch-size", "b", batchSize, "size of the batch to save to the database")
	transformCmd.Flags().BoolVarP(&args.PostgresLogged, "logged", "l", args.PostgresLogged, "avoids the disk overhead but writes slowly to the tables")
	return transformCmd
}
