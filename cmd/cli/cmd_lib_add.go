package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

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
}

func addLibraryFile(store msgstore.ScriptStore, filepath string, filename string) error {
	fullname := path.Join(filepath, filename)
	content, err := os.ReadFile(fullname)
	if err != nil {
		return fmt.Errorf("failed to read %s: %v", filename, err)
	}

	p := strings.Replace(strings.Replace(filename, filepath, "", 1), path.Ext(filename), "", 1)
	log.Debugf("loading library %s", p)
	return store.AddLibrary(context.Background(), string(content), p)
}

func libAddRun(cmd *cobra.Command, args []string) {
	store, err := msgstore.StoreByName(cmd.Flag("backend").Value.String(), cmd.Flag("etcdurls").Value.String())
	if err != nil {
		log.Errorf("failed to create store: %v", err)
		return
	}

	var count int
	for _, arg := range args {
		fields := log.Fields{
			"Path": arg,
		}

		stat, err := os.Stat(arg)
		if err != nil {
			log.WithFields(fields).Errorf("failed to stat %s: %v", arg, err)
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
						err = addLibraryFile(store, arg, filename)
						if err != nil {
							return fmt.Errorf("failed add library file: %v", err)
						}
						count++
					}

					return nil
				})
			} else {
				entries, err := os.ReadDir(arg)
				if err != nil {
					log.WithFields(fields).Errorf("failed to read directory: %v", err)
					return
				}

				for _, e := range entries {
					if path.Ext(e.Name()) == ".lua" {
						err = addLibraryFile(store, arg, e.Name())
						if err != nil {
							log.WithFields(fields).WithField("filename", e.Name()).Errorf("failed add library file: %v", err)
							return
						}
						count++
					}
				}

			}
		} else {
			err = addLibraryFile(store, "", arg)
			if err != nil {
				log.WithFields(fields).Errorf("failed add library file: %v", err)
				return
			}

		}
	}

	fmt.Printf("Added %d libraries\n", count)
}
