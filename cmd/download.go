package cmd

import (
	"codeberg.org/cuducos/minha-receita/transform"
	"github.com/spf13/cobra"
)

const downloadHelper = `
Downloads a monthly CNPJ snapshot from the Receita Federal into the data
directory, ready for the transform/update commands.

The CNPJ files are bundled into a YYYY-MM.zip and the tax-regime files
(entidades-*.zip) are placed next to it. Data comes from two public Nextcloud
shares; their tokens rotate occasionally, so they can be overridden with the
RECEITA_CNPJ_SHARE_URL and RECEITA_REGIME_SHARE_URL environment variables
(paste the .../index.php/s/<token> URL from a browser).`

var (
	downloadMonth      string
	downloadParallel   int
	downloadSkipMain   bool
	downloadSkipRegime bool
	downloadList       bool
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Downloads a monthly CNPJ snapshot from the Receita Federal",
	Long:  downloadHelper,
	RunE: func(_ *cobra.Command, _ []string) error {
		return transform.Download(dir, downloadMonth, downloadParallel, downloadSkipMain, downloadSkipRegime, downloadList)
	},
}

func downloadCLI() *cobra.Command {
	downloadCmd.Flags().StringVarP(&downloadMonth, "month", "y", "", "snapshot month (YYYY-MM); default is the latest available")
	downloadCmd.Flags().IntVar(&downloadParallel, "parallel", 4, "number of parallel downloads")
	downloadCmd.Flags().BoolVar(&downloadSkipMain, "skip-main", false, "skip the CNPJ files, download only the tax-regime files")
	downloadCmd.Flags().BoolVar(&downloadSkipRegime, "skip-regime", false, "skip the tax-regime files, download only the CNPJ files")
	downloadCmd.Flags().BoolVar(&downloadList, "list", false, "only list what would be downloaded, without downloading")
	return downloadCmd
}
