package main

import (
	"context"
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

func addLibraryFile(store msgstore.ScriptStore, argName, fullname string) error {
	r := script.ScriptReader{}
	err := r.ReadFile(fullname)
	if err != nil {
		return fmt.Errorf("failed to read script: %v", err)
	}

	content, err := os.ReadFile(fullname)
	if err != nil {
		return fmt.Errorf("failed to read %s: %v", fullname, err)
	}

	name := r.Script.Name
	if argName != "" {
		name = argName
	}

	log.Debugf("loading library %s", name)
	return store.AddLibrary(context.Background(), string(content), name)
}

func libAddRun(cmd *cobra.Command, args []string) {
	store, err := msgstore.StoreByName(cmd.Flag("backend").Value.String(), cmd.Flag("etcdurls").Value.String())
	if err != nil {
		log.Errorf("failed to create store: %v", err)
		return
	}

	var count int
	for _, arg := range args {
		stat, err := os.Stat(arg)
		if err != nil {
			cmd.PrintErrf("failed to stat %s: %v", arg, err)
			return
		}

		recursive, err := cmd.Flags().GetBool("recursive")
		if stat.IsDir() {
			if recursive {
				fsys := os.DirFS(arg)
				err = fs.WalkDir(fsys, ".", func(filename string, d os.DirEntry, err error) error {
					if err != nil {
						return err
					}

					if path.Ext(filename) == ".lua" {
						fullname := path.Join(arg, filename)
						err = addLibraryFile(store, cmd.Flag("name").Value.String(), fullname)
						if err != nil {
							return fmt.Errorf("failed add library file: %v", err)
						}
						count += 1
					}

					return nil
				})
			} else {
				entries, err := os.ReadDir(arg)
				if err != nil {
					cmd.PrintErrf("failed to read directory: %v", err)
					return
				}

				for _, e := range entries {
					if path.Ext(e.Name()) == ".lua" {
						fullname := path.Join(arg, e.Name())
						err = addLibraryFile(store, cmd.Flag("name").Value.String(), fullname)
						if err != nil {
							cmd.PrintErrf("failed add library file %s: %v", e.Name(), err)
							return
						}
						count += 1
					}
				}

			}
		} else {
			err = addLibraryFile(store, cmd.Flag("name").Value.String(), arg)
			if err != nil {
				cmd.PrintErrf("failed add library file: %v", err)
				return
			}
			count += 1
		}
	}

	cmd.Printf("Added %d libraries\n", count)
}
