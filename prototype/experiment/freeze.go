package experiment

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FrozenPaths are the inputs the pre-registration covers (§3.6): rule logic,
// corpus generation and transformation (including the noise defaults), and the
// prediction table. Anything else may change without invalidating the freeze.
var FrozenPaths = []string{"rules", "experiment", "eval/predictions.md"}

// HashTree returns sha256 per .go/.md file under root/FrozenPaths, keyed by
// slash-separated relative path. Test files are excluded: they do not change
// what the sweep computes.
func HashTree(root string) (map[string]string, error) {
	out := map[string]string{}
	for _, frozen := range FrozenPaths {
		err := filepath.WalkDir(filepath.Join(root, frozen), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if ext := filepath.Ext(path); ext != ".go" && ext != ".md" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// WriteManifest writes "hash  path" lines in path order, the sha256sum format.
func WriteManifest(path string, hashes map[string]string) error {
	paths := make([]string, 0, len(hashes))
	for p := range hashes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var b strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&b, "%s  %s\n", hashes[p], p)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// ManifestDigest is the single identifier quoted in the thesis: sha256 over the
// manifest file bytes.
func ManifestDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// VerifyFrozen fails if any frozen file was added, removed or edited since the
// manifest was written. The sweep calls it before producing any held-out result.
func VerifyFrozen(root, manifestPath string) error {
	file, err := os.Open(manifestPath)
	if err != nil {
		return err
	}
	defer file.Close()
	expected := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			return fmt.Errorf("freeze: malformed manifest line %q", scanner.Text())
		}
		expected[fields[1]] = fields[0]
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	actual, err := HashTree(root)
	if err != nil {
		return err
	}
	var problems []string
	for p, h := range expected {
		switch got, ok := actual[p]; {
		case !ok:
			problems = append(problems, "missing "+p)
		case got != h:
			problems = append(problems, "modified "+p)
		}
	}
	for p := range actual {
		if _, ok := expected[p]; !ok {
			problems = append(problems, "added "+p)
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("freeze: frozen inputs changed since manifest: %s", strings.Join(problems, "; "))
	}
	return nil
}
