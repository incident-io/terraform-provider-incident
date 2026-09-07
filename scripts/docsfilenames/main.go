// Command docsfilenames restores the provider prefix that tfplugindocs strips
// from the docs page of any resource whose name already starts with it.
//
// tfplugindocs names each page after the resource with a single "incident_"
// stripped, so incident_alert_route becomes docs/resources/alert_route.md. The
// Terraform Registry sorts its docs nav by that filename, but labels each entry
// with the resource name, adding "incident_" back only when the filename does
// not already start with it.
//
// Resources whose name repeats the prefix land badly: incident_incident_role
// generates incident_role.md, which the registry labels "incident_role" - not a
// resource we have - and sorts under "i", between escalation_path and policy
// rather than alphabetically alongside its siblings. Naming those pages after
// the full resource name fixes the label and the ordering, and the registry
// serves prefixed filenames happily (see confluentinc/confluent and
// cyrilgdn/postgresql, which name every page that way).
//
// This runs from go:generate after tfplugindocs, which wipes and rewrites the
// docs subdirectories on each run.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const prefix = "incident_"

var (
	dirs      = []string{"docs/resources", "docs/data-sources"}
	pageTitle = regexp.MustCompile(`(?m)^page_title: "(incident_[a-z0-9_]+) `)
)

func main() {
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			log.Fatalf("reading %s: %v", dir, err)
		}

		for _, entry := range entries {
			stem, ok := strings.CutSuffix(entry.Name(), ".md")
			// Only pages whose filename already starts with the prefix can be
			// mislabelled, and only those need renaming.
			if !ok || !strings.HasPrefix(stem, prefix) {
				continue
			}

			path := filepath.Join(dir, entry.Name())
			name, err := resourceName(path)
			if err != nil {
				log.Fatal(err)
			}
			if name == stem {
				continue // already named after the resource
			}
			if name != prefix+stem {
				log.Fatalf("%s: expected page for %q, found %q", path, prefix+stem, name)
			}

			renamed := filepath.Join(dir, name+".md")
			if err := os.Rename(path, renamed); err != nil {
				log.Fatalf("renaming %s: %v", path, err)
			}
			fmt.Printf("renamed %s to %s\n", path, renamed)
		}
	}
}

// resourceName reads the resource this page documents out of its front matter.
func resourceName(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}

	match := pageTitle.FindSubmatch(contents)
	if match == nil {
		return "", fmt.Errorf("%s: no page_title in front matter", path)
	}

	return string(match[1]), nil
}
