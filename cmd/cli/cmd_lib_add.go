package main

import (
	"fmt"
	"io/fs"
	"os"
	"path"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/numkem/msgscript"
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
	store, err := msgstore.StoreByName(cmd.Flag("backend").Value.String(), cmd.Flag("etcdurls").Value.String(), "", "")
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
		err = store.AddLibrary(cmd.Context(), lib.Content, lib.Name)
		if err != nil {
			cmd.PrintErrln(fmt.Errorf("failed to add library %s to the store: %v", lib.Name, err))
			return
		}
	}

	cmd.Printf("Added %d libraries\n", len(libraries))
}

func parseDirsForLibraries(dirnames []string, recursive bool) ([]*msgscript.Script, error) {
	var scripts []*msgscript.Script
	for _, fname := range dirnames {
		stat, err := os.Stat(fname)
		if err != nil {
			return nil, fmt.Errorf("failed to stat %s: %v", fname, err)
		}

		if stat.IsDir() {
			if recursive {
				fsys := os.DirFS(fname)
				err = fs.WalkDir(fsys, ".", func(filename string, d os.DirEntry, err error) error {
					if err != nil {
						return err
					}

					if path.Ext(filename) == ".lua" {
						fullname := path.Join(fname, filename)
						s, err := msgscript.ReadFile(fullname)
						if err != nil {
							return fmt.Errorf("failed to read script %s: %v", fullname, err)
						}

						scripts = append(scripts, s)
					}

					return nil
				})
			} else {
				entries, err := os.ReadDir(fname)
				if err != nil {
					return nil, fmt.Errorf("failed to read directory: %v", err)
				}

				for _, e := range entries {
					if path.Ext(e.Name()) == ".lua" {
						fullname := path.Join(fname, e.Name())

						s, err := msgscript.ReadFile(fname)
						if err != nil {
							return nil, fmt.Errorf("failed to read script %s: %v", fullname, err)
						}

						scripts = append(scripts, s)
					}
				}

			}
		} else {
			s, err := msgscript.ReadFile(fname)
			if err != nil {
				return nil, fmt.Errorf("failed to read file %s: %v", fname, err)
			}

			if s.Name == "" {
				return nil, fmt.Errorf("script at %s requires to have the 'name' header", fname)
			}

			scripts = append(scripts, s)
		}
	}

	return scripts, nil
}
