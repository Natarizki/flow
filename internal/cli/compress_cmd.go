package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Natarizki/flow/internal/compression"
)

func RegisterCompressCommands(root *cobra.Command) {
	var level int
	var output string
	var recursive bool

	compressCmd := &cobra.Command{
		Use:   "compress <file>",
		Short: "Compress a file into .flow format",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := args[0]

			info, err := os.Stat(input)
			if err != nil {
				return fmt.Errorf("cannot access %s: %w", input, err)
			}

			if info.IsDir() {
				if !recursive {
					return fmt.Errorf("%s is a directory, use --recursive to compress folders", input)
				}
				return compressFolder(input, level, output)
			}
			return compressFile(input, level, output)
		},
	}
	compressCmd.Flags().IntVar(&level, "level", 0, "quantization level 0-4")
	compressCmd.Flags().StringVar(&output, "output", "", "output file/folder path")
	compressCmd.Flags().BoolVar(&recursive, "recursive", false, "compress folder recursively")

	decompressCmd := &cobra.Command{
		Use:   "decompress <file.flow.N>",
		Short: "Decompress a .flow file back to original",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return decompressFile(args[0], output)
		},
	}
	decompressCmd.Flags().StringVar(&output, "output", "", "output file path")

	convertCmd := &cobra.Command{
		Use:   "convert <file.flow.N>",
		Short: "Convert a .flow file to a different quantization level",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			toLevel, _ := cmd.Flags().GetInt("to")
			return convertFile(args[0], toLevel, output)
		},
	}
	convertCmd.Flags().Int("to", 0, "target level 0-4")
	convertCmd.Flags().StringVar(&output, "output", "", "output file path")
	convertCmd.MarkFlagRequired("to")

	infoCmd := &cobra.Command{
		Use:   "info <file.flow.N>",
		Short: "Show metadata of a .flow file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return showFlowInfo(args[0])
		},
	}

	root.AddCommand(compressCmd, decompressCmd, convertCmd, infoCmd)
}

func compressFile(input string, level int, output string) error {
	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}

	contentType := guessContentType(input)
	flowFile, err := compression.Compress(data, compression.QuantLevel(level), input, contentType)
	if err != nil {
		return err
	}

	encoded, err := compression.Encode(flowFile)
	if err != nil {
		return err
	}

	if output == "" {
		output = input + compression.ExtensionFor(compression.QuantLevel(level))
	}
	if err := os.WriteFile(output, encoded, 0644); err != nil {
		return err
	}

	ratio := flowFile.Metadata.Compression.Ratio
	fmt.Printf("compressed %s -> %s (level %d, ratio %.2f, %d -> %d bytes)\n",
		input, output, level, ratio, len(data), len(encoded))
	return nil
}

func compressFolder(inputDir string, level int, outputDir string) error {
	if outputDir == "" {
		outputDir = inputDir + "-compressed"
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	count := 0
	err := filepathWalk(inputDir, func(path string, isDir bool) error {
		if isDir {
			return nil
		}
		relPath := strings.TrimPrefix(path, inputDir)
		outPath := outputDir + relPath + compression.ExtensionFor(compression.QuantLevel(level))
		os.MkdirAll(dirOf(outPath), 0755)

		if err := compressFile(path, level, outPath); err != nil {
			fmt.Printf("skip %s: %v\n", path, err)
			return nil
		}
		count++
		return nil
	})
	if err != nil {
		return err
	}

	fmt.Printf("compressed %d files -> %s\n", count, outputDir)
	return nil
}

func decompressFile(input string, output string) error {
	raw, err := os.ReadFile(input)
	if err != nil {
		return err
	}

	flowFile, err := compression.Decode(raw)
	if err != nil {
		return err
	}

	decompressed, err := decompressData(flowFile)
	if err != nil {
		return err
	}

	if output == "" {
		output = strings.TrimSuffix(input, compression.ExtensionFor(flowFile.Level))
	}
	if err := os.WriteFile(output, decompressed, 0644); err != nil {
		return err
	}

	fmt.Printf("decompressed %s -> %s (%d bytes)\n", input, output, len(decompressed))
	return nil
}

func convertFile(input string, toLevel int, output string) error {
	raw, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	flowFile, err := compression.Decode(raw)
	if err != nil {
		return err
	}
	original, err := decompressData(flowFile)
	if err != nil {
		return err
	}

	newFlow, err := compression.Compress(original, compression.QuantLevel(toLevel),
		flowFile.Metadata.Original.URL, flowFile.Metadata.Original.ContentType)
	if err != nil {
		return err
	}
	encoded, err := compression.Encode(newFlow)
	if err != nil {
		return err
	}

	if output == "" {
		base := strings.TrimSuffix(input, compression.ExtensionFor(flowFile.Level))
		output = base + compression.ExtensionFor(compression.QuantLevel(toLevel))
	}
	if err := os.WriteFile(output, encoded, 0644); err != nil {
		return err
	}

	fmt.Printf("converted %s (level %d -> %d) -> %s\n", input, flowFile.Level, toLevel, output)
	return nil
}

func showFlowInfo(input string) error {
	raw, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	flowFile, err := compression.Decode(raw)
	if err != nil {
		return err
	}

	m := flowFile.Metadata
	fmt.Printf("File: %s\n", input)
	fmt.Printf("Level: %d (quality %d%%)\n", flowFile.Level, flowFile.Quality)
	fmt.Printf("Original size: %d bytes\n", flowFile.OriginalSize)
	fmt.Printf("Compressed size: %d bytes\n", flowFile.CompressedSize)
	fmt.Printf("Ratio: %.2f\n", m.Compression.Ratio)
	fmt.Printf("Methods: %s\n", strings.Join(m.Compression.Methods, ", "))
	fmt.Printf("Content-Type: %s\n", m.Original.ContentType)
	fmt.Printf("Checksum: %s...\n", m.Original.Checksum[:16])
	return nil
}
