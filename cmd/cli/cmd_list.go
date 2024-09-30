package main

import (
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	msgstore "github.com/numkem/msgscript/store"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "list all the scripts registered in the store",
	Run:     listCmdRun,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func listCmdRun(cmd *cobra.Command, args []string) {
	scriptStore, err := msgstore.StoreByName(cmd.Flag("backend").Value.String(), cmd.Flag("etcdurls").Value.String())
	if err != nil {
		cmd.PrintErrf("failed to get script store: %v", err)
		return
	}

	subjectScriptNames := make(map[string][]string)
	subjects, err := scriptStore.ListSubjects(cmd.Context())
	if err != nil {
		cmd.PrintErrf("failed to get subjects from store: %v", err)
		return
	}

	for _, subject := range subjects {
		scripts, err := scriptStore.GetScripts(cmd.Context(), subject)
		if err != nil {
			cmd.PrintErrf("failed to get scripts for subject %s: %v", subject, err)
			return
		}

		for name := range scripts {
			_, found := subjectScriptNames[subject]
			if !found {
				subjectScriptNames[subject] = []string{}
			}
			subjectScriptNames[subject] = append(subjectScriptNames[subject], name)
		}
	}

	t := table.NewWriter()
	t.SetOutputMirror(cmd.OutOrStdout())
	t.SetStyle(table.StyleLight)
	t.Style().Options.DrawBorder = false
	t.Style().Options.SeparateRows = false
	t.Style().Options.SeparateColumns = false
	t.Style().Options.SeparateHeader = false
	t.Style().Options.SeparateFooter = false

	t.AppendHeader(table.Row{"Subject", "Name"})

	for subject, names := range subjectScriptNames {
		for _, name := range names {
			ss := strings.Split(name, "/")
			n := ss[len(ss)-1]
			t.AppendRow(table.Row{subject, n})
		}
	}

	t.Render()
}
