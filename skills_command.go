package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/skills"
	"github.com/spf13/cobra"
)

func newSkillsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "skills",
		Short: "Print the agent usage guide.",
		Long:  "🧩 Print the bundled OTel Desktop Viewer agent usage guide.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := io.Copy(cmd.OutOrStdout(), strings.NewReader(skills.Guide())); err != nil {
				return fmt.Errorf("write skill: %w", err)
			}
			return nil
		},
	}
}
