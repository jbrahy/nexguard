package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jbrahy/AntiVirus/internal/config"
	"github.com/jbrahy/AntiVirus/internal/store"
	"github.com/spf13/cobra"
)

type dbContextKey struct{}

var dbPathFlag string
var quarantineDirFlag string

var rootCmd = &cobra.Command{
	Use:           "avtool",
	Short:         "Hash-based antivirus scanner and watcher for macOS",
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		path := dbPathFlag
		if path == "" {
			p, err := config.DBPath()
			if err != nil {
				return err
			}
			path = p
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("creating db directory: %w", err)
		}
		db, err := store.Open(path)
		if err != nil {
			return err
		}
		cmd.SetContext(context.WithValue(cmd.Context(), dbContextKey{}, db))
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		if db := dbFromCmd(cmd); db != nil {
			return db.Close()
		}
		return nil
	},
}

func dbFromCmd(cmd *cobra.Command) *sql.DB {
	db, _ := cmd.Context().Value(dbContextKey{}).(*sql.DB)
	return db
}

// version is overwritten at release-build time with
// -ldflags "-X main.version=vX.Y.Z". It stays "dev" for a plain `go build`
// so an unstamped binary never claims to be a release.
var version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print avtool's version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), "avtool "+version)
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPathFlag, "db-path", "", "path to avtool's SQLite database (default: <OS config dir>/avtool/avtool.db; see README)")
	rootCmd.PersistentFlags().StringVar(&quarantineDirFlag, "quarantine-dir", "", "path to quarantine directory (default: <OS config dir>/avtool/quarantine; see README)")
	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
