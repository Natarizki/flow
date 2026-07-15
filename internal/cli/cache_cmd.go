package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Natarizki/flow/internal/compression"
)

func RegisterCacheCommands(root *cobra.Command) {
	cacheCmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage local cache",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List cached entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			var entries []map[string]interface{}
			if err := apiGet("/api/cache", &entries); err != nil {
				return err
			}
			for _, e := range entries {
				fmt.Printf("%-40s %10v bytes  level:%v\n", e["hash"], e["size"], e["quant_level"])
			}
			return nil
		},
	}

	cleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean cache by age or size",
		RunE: func(cmd *cobra.Command, args []string) error {
			olderThan, _ := cmd.Flags().GetString("older-than")
			if olderThan != "" {
				d, err := parseDuration(olderThan)
				if err != nil {
					return err
				}
				var resp map[string]int
				if err := apiPost("/api/cache/clean", map[string]interface{}{"older_than_seconds": int(d.Seconds())}, &resp); err != nil {
					return err
				}
				fmt.Printf("cleaned %d stale entries\n", resp["removed"])
			}
			return nil
		},
	}
	cleanCmd.Flags().String("older-than", "", "e.g. 7d, 24h")

	exportCmd := &cobra.Command{
		Use:   "export <destination>",
		Short: "Export cache to a backup file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := apiPost("/api/cache/export", map[string]string{"destination": args[0]}, nil); err != nil {
				return err
			}
			fmt.Printf("cache exported to %s\n", args[0])
			return nil
		},
	}

	importCmd := &cobra.Command{
		Use:   "import <source>",
		Short: "Import cache from a backup file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := apiPost("/api/cache/import", map[string]string{"source": args[0]}, nil); err != nil {
				return err
			}
			fmt.Printf("cache imported from %s\n", args[0])
			return nil
		},
	}

	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show cache statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			var stats map[string]interface{}
			if err := apiGet("/api/stats", &stats); err != nil {
				return err
			}
			fmt.Printf("Peers connected: %v\n", stats["peer_count"])
			fmt.Printf("Cached entries: %v\n", stats["cache_count"])
			fmt.Printf("Cache size: %v bytes\n", stats["cache_size"])
			return nil
		},
	}

	cacheCmd.AddCommand(listCmd, cleanCmd, exportCmd, importCmd, statsCmd)
	root.AddCommand(cacheCmd)
}

// --- helpers dipakai bareng compress_cmd.go ---

func guessContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".html", ".htm":
		return "text/html"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}

func decompressData(f *compression.FlowFile) ([]byte, error) {
	// level image pakai jpeg encode langsung (bukan zstd), jadi kalau
	// content-type gambar, data udah final -- nggak perlu zstd decode.
	if strings.HasPrefix(f.Metadata.Original.ContentType, "image/") && f.Level != compression.LevelLossless {
		return f.Data, nil
	}
	buf, err := compression.DecodeBuffer(f.Data)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(bytes.NewReader(buf.Bytes()))
}

func filepathWalk(root string, fn func(path string, isDir bool) error) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return fn(path, info.IsDir())
	})
}

func dirOf(path string) string {
	return filepath.Dir(path)
}

func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
