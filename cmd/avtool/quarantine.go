package main

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/jbrahy/AntiVirus/internal/config"
	"github.com/jbrahy/AntiVirus/internal/quarantine"
	"github.com/spf13/cobra"
)

var quarantineListJSONFlag bool

var quarantineCmd = &cobra.Command{
	Use:   "quarantine",
	Short: "Manage quarantined files",
}

func getQuarantineDir() (string, error) {
	if quarantineDirFlag != "" {
		return quarantineDirFlag, nil
	}
	return config.QuarantineDir()
}

var quarantineListCmd = &cobra.Command{
	Use:   "list",
	Short: "List quarantine records",
	RunE: func(cmd *cobra.Command, args []string) error {
		records, err := quarantine.List(dbFromCmd(cmd))
		if err != nil {
			return err
		}

		if quarantineListJSONFlag {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(records)
		}

		if len(records) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no quarantine records")
			return nil
		}
		for _, r := range records {
			fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\trestored=%v\n", r.ID, r.OriginalPath, r.Hash, r.Restored)
		}
		return nil
	},
}

var quarantineRestoreCmd = &cobra.Command{
	Use:   "restore <id>",
	Short: "Restore a quarantined file to its original path",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid id %q: %w", args[0], err)
		}
		qDir, err := getQuarantineDir()
		if err != nil {
			return err
		}
		if err := quarantine.Restore(dbFromCmd(cmd), qDir, id); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "restored %d\n", id)
		fmt.Fprintln(cmd.OutOrStdout(), "note: file permissions were reset when quarantined; you may need to chmod it")
		return nil
	},
}

var quarantinePurgeCmd = &cobra.Command{
	Use:   "purge <id>",
	Short: "Permanently delete a quarantined file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid id %q: %w", args[0], err)
		}
		qDir, err := getQuarantineDir()
		if err != nil {
			return err
		}
		if err := quarantine.Purge(dbFromCmd(cmd), qDir, id); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "purged %d\n", id)
		return nil
	},
}

func init() {
	quarantineListCmd.Flags().BoolVar(&quarantineListJSONFlag, "json", false, "Output quarantine records in JSON format")
	quarantineCmd.AddCommand(quarantineListCmd, quarantineRestoreCmd, quarantinePurgeCmd)
	rootCmd.AddCommand(quarantineCmd)
}
