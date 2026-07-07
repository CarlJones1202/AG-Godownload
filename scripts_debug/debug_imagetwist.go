package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
)

type FileInfo struct {
	Path   string
	Size   int64
	SHA256 string
}

func main() {
	var fileInfos []FileInfo
	hashCounts := make(map[string]int)
	hashPaths := make(map[string][]string)

	err := filepath.Walk("uploads", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		
		// Let's filter only images (jpg, png, webp, gif)
		ext := filepath.Ext(path)
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" && ext != ".gif" {
			return nil
		}

		// Calculate sha256
		hash := sha256.New()
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		io.Copy(hash, f)
		f.Close()
		hashStr := hex.EncodeToString(hash.Sum(nil))

		hashCounts[hashStr]++
		hashPaths[hashStr] = append(hashPaths[hashStr], path)

		fileInfos = append(fileInfos, FileInfo{
			Path:   path,
			Size:   info.Size(),
			SHA256: hashStr,
		})
		return nil
	})

	if err != nil {
		log.Fatalf("walk failed: %v", err)
	}

	fmt.Printf("Scanned %d files in uploads\n", len(fileInfos))

	// Find top duplicate hashes
	type HashCount struct {
		Hash  string
		Count int
		Size  int64
		Paths []string
	}
	var duplicates []HashCount
	for h, c := range hashCounts {
		if c > 1 {
			var sz int64
			for _, fi := range fileInfos {
				if fi.SHA256 == h {
					sz = fi.Size
					break
				}
			}
			duplicates = append(duplicates, HashCount{Hash: h, Count: c, Size: sz, Paths: hashPaths[h]})
		}
	}
	
	sort.Slice(duplicates, func(i, j int) bool {
		return duplicates[i].Count > duplicates[j].Count
	})

	fmt.Println("\n--- Top 15 Duplicate Hashes in uploads/ ---")
	for i := 0; i < len(duplicates) && i < 15; i++ {
		d := duplicates[i]
		fmt.Printf("Count: %d, Size: %d, SHA256: %s\n", d.Count, d.Size, d.Hash)
		for j := 0; j < len(d.Paths) && j < 3; j++ {
			fmt.Printf("  -> %s\n", d.Paths[j])
		}
		if len(d.Paths) > 3 {
			fmt.Println("  -> ...")
		}
	}
}
