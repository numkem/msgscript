package main

import (
	"fmt"
	"io/fs"
	"os"
	"path"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/numkem/msgscript/script"
	msgstore "github.com/numkem/msgscript/store"
)

var libAddCmd = &cobra.Command{
	Use:   "add",
	Short: "add libraries",
	Long:  "add libraries either as a single file or as a directory. Only files ending in .lua will be added",
	Run:   libAddRun,
}

func init() {
	libCmd.AddCommand(libAddCmd)

	libAddCmd.PersistentFlags().BoolP("recursive", "r", false, "Add files in path recursively")
	libAddCmd.PersistentFlags().String("name", "n", "Name of the library")

	libAddCmd.MarkFlagRequired("name")
}

func libAddRun(cmd *cobra.Command, args []string) {
	store, err := msgstore.StoreByName(cmd.Flag("backend").Value.String(), cmd.Flag("etcdurls").Value.String())
	if err != nil {
		log.Errorf("failed to create store: %v", err)
		return
	}

	recursive, err := cmd.Flags().GetBool("recursive")
	if err != nil {
		cmd.PrintErrln(fmt.Errorf("failed parse recursive flag: %v", err))
		return
	}

	libraries, err := parseDirsForLibraries(args, recursive)
	if err != nil {
		cmd.PrintErrln(fmt.Errorf("failed to parse directories for librairies: %v", err))
		return
	}

	for _, lib := range libraries {
		err = store.AddLibrary(cmd.Context(), string(lib.Content), lib.Name)
		if err != nil {
			cmd.PrintErrln(fmt.Errorf("failed to add library %s to the store: %v", lib.Name, err))
			return
		}
	}

	cmd.Printf("Added %d libraries\n", len(libraries))
}

func parseDirsForLibraries(dirnames []string, recursive bool) ([]*script.Script, error) {
	var scripts []*script.Script
	for _, dir := range dirnames {
		stat, err := os.Stat(dir)
		if err != nil {
			return nil, fmt.Errorf("failed to stat %s: %v", dir, err)
		}

		if stat.IsDir() {
			if recursive {
				fsys := os.DirFS(dir)
				err = fs.WalkDir(fsys, ".", func(filename string, d os.DirEntry, err error) error {
					if err != nil {
						return err
					}

					if path.Ext(filename) == ".lua" {
						s := new(script.Script)
						fullname := path.Join(dir, filename)
						err = s.ReadFile(fullname)
						if err != nil {
							return fmt.Errorf("failed to read script %s: %v", fullname, err)
						}

						scripts = append(scripts, s)
					}

					return nil
				})
			} else {
				entries, err := os.ReadDir(dir)
				if err != nil {
					return nil, fmt.Errorf("failed to read directory: %v", err)
				}

				for _, e := range entries {
					if path.Ext(e.Name()) == ".lua" {
						fullname := path.Join(dir, e.Name())

						s := new(script.Script)
						err = s.ReadFile(fullname)
						if err != nil {
							return nil, fmt.Errorf("failed to read script %s: %v", fullname, err)
						}

						scripts = append(scripts, s)
					}
				}

			}
		} else {
			s := new(script.Script)
			err = s.ReadFile(dir)
			if err != nil {
				return nil, fmt.Errorf("failed to read file %s: %v", dir, err)
			}

			scripts = append(scripts, s)
		}
	}

	return scripts, nil
}
