// freeze writes a SHA-256 manifest and tar.gz of the frozen inputs under
// eval/freeze/<date>/. Never edit the result; a correction gets a new date.
package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gimz12/vehicle-registry-portal/prototype/experiment"
)

func main() {
	root := flag.String("root", ".", "prototype module root")
	date := flag.String("date", time.Now().UTC().Format("2006-01-02"), "freeze date (directory name)")
	flag.Parse()

	dir := filepath.Join(*root, "eval", "freeze", *date)
	if _, err := os.Stat(dir); err == nil {
		log.Fatalf("freeze: %s already exists; a freeze is never overwritten", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	hashes, err := experiment.HashTree(*root)
	if err != nil {
		log.Fatal(err)
	}
	manifest := filepath.Join(dir, "MANIFEST.sha256")
	if err := experiment.WriteManifest(manifest, hashes); err != nil {
		log.Fatal(err)
	}
	if err := writeArchive(*root, hashes, filepath.Join(dir, "frozen.tar.gz")); err != nil {
		log.Fatal(err)
	}
	digest, err := experiment.ManifestDigest(manifest)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("frozen %d files into %s\nmanifest sha256 %s\n", len(hashes), dir, digest)
}

func writeArchive(root string, hashes map[string]string, out string) error {
	file, err := os.Create(out)
	if err != nil {
		return err
	}
	defer file.Close()
	gz := gzip.NewWriter(file)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	paths := make([]string, 0, len(hashes))
	for p := range hashes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		info, err := os.Stat(filepath.Join(root, p))
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = p
		header.ModTime = time.Unix(0, 0) // reproducible archive bytes
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		src, err := os.Open(filepath.Join(root, p))
		if err != nil {
			return err
		}
		_, err = io.Copy(tw, src)
		src.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
