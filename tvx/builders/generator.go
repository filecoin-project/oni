package builders

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/filecoin-project/oni/tvx/schema"
)

// Generator is a batch generator and organizer of test vectors.
//
// Test vector scripts are simple programs (main function). Test vector scripts
// can delegate to the Generator to handle the execution, reporting and capture
// of emitted test vectors into files.
//
// Generator supports the following CLI flags:
//
//  -o <directory>
//		directory where test vector JSON files will be saved.
//
//  -f <regex>
//		regex filter to select a subset of vectors to execute; matched against
//	 	the vector's ID.
//
// Scripts can bundle test vectors into "groups". The generator will execute
// each group in parallel, and will write each vector in a file:
// <output_dir>/<group>-<sequential>.json
type Generator struct {
	OutputPath string
	Filter     *regexp.Regexp

	wg sync.WaitGroup
}

type MessageVectorGenItem struct {
	Metadata *schema.Metadata
	Func     func(*Builder)
}

func NewGenerator() *Generator {
	// Consume CLI parameters.
	var (
		outputDir = flag.String("o", "", "output directory")
		filter    = flag.String("f", "", "filter regex for test vectors to generate")
	)

	flag.Parse()

	ret := new(Generator)

	// Output directory is compulsory.
	if *outputDir == "" {
		fmt.Printf("output directory not specified\n")
		os.Exit(1)
	}

	err := ensureDirectory(*outputDir)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	ret.OutputPath = *outputDir

	// If a filter has been provided, compile it into a regex.
	if *filter != "" {
		exp, err := regexp.Compile(*filter)
		if err != nil {
			fmt.Printf("supplied regex %s is invalid: %s\n", *filter, err)
			os.Exit(1)
		}
		ret.Filter = exp
	}

	return ret
}

func (g *Generator) Wait() {
	g.wg.Wait()
}

func (g *Generator) MessageVectorGroup(group string, vectors ...MessageVectorGenItem) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()

		var wg sync.WaitGroup
		for i, item := range vectors {
			if id := item.Metadata.ID; g.Filter != nil && !g.Filter.MatchString(id) {
				fmt.Printf("skipping %s\n", id)
				continue
			}

			file := filepath.Join(g.OutputPath, fmt.Sprintf("%s-%d.json", group, i))
			out, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE, 0644)
			if err != nil {
				fmt.Printf("failed to write to file %s: %s\n", file, err)
				return
			}
			wg.Add(1)
			go func(item *MessageVectorGenItem) {
				g.generateOne(out, item)
				wg.Done()
			}(&item)
		}

		wg.Wait()
	}()
}

func (g *Generator) generateOne(w io.Writer, b *MessageVectorGenItem) {
	fmt.Printf("generating test vector: %s\n", b.Metadata.ID)

	vector := MessageVector(b.Metadata)

	// TODO: currently if an assertion fails, we call os.Exit(1), which
	//  aborts all ongoing vector generations. The Asserter should
	//  call runtime.Goexit() instead so only that goroutine is
	//  cancelled. The assertion error must bubble up somehow.
	b.Func(vector)

	buf := new(bytes.Buffer)
	vector.Finish(buf)

	// reparse and reindent.
	indented := new(bytes.Buffer)
	if err := json.Indent(indented, buf.Bytes(), "", "\t"); err != nil {
		fmt.Printf("failed to indent json: %s\n", err)
	}

	if _, err := w.Write(indented.Bytes()); err != nil {
		fmt.Printf("failed to write to output: %s\n", err)
		return
	}
}

// ensureDirectory checks if the provided path is a directory. If yes, it
// returns nil. If the path doesn't exist, it creates the directory and
// returns nil. If the path is not a directory, or another error occurs, an
// error is returned.
func ensureDirectory(path string) error {
	switch stat, err := os.Stat(path); {
	case os.IsNotExist(err):
		// create directory.
		fmt.Printf("creating directory %s\n", path)
		err := os.MkdirAll(path, 0700)
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %s\n", path, err)
		}

	case err == nil && !stat.IsDir():
		return fmt.Errorf("path %s exists, but it's not a directory", path)

	case err != nil:
		return fmt.Errorf("failed to stat directory %s: %w", path, err)
	}
	return nil
}
