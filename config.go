package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	inputPaths              []string
	outputPath              string
	outputDir               string
	black, noFrame, noModel bool
	preserveExif            bool
	frameRatio              float64
	quality                 int
}

type inputPathsFlag []string

func (paths *inputPathsFlag) String() string { return strings.Join(*paths, ", ") }
func (paths *inputPathsFlag) Set(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("input path cannot be empty")
	}
	*paths = append(*paths, path)
	return nil
}

func parseConfig(args []string, output io.Writer) (Config, error) {
	var cfg Config
	var paths inputPathsFlag
	fs := flag.NewFlagSet("go-exiframe", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.Var(&paths, "f", "Path to an image file; repeat -f to process multiple images")
	fs.StringVar(&cfg.outputPath, "o", "", "Output JPEG path (single input only)")
	fs.StringVar(&cfg.outputDir, "out-dir", ".", "Output directory")
	fs.BoolVar(&cfg.black, "black", false, "Use black frame (default white)")
	fs.BoolVar(&cfg.noFrame, "no-frame", false, "Draw no border around the image")
	fs.BoolVar(&cfg.noModel, "no-model", false, "Omit camera and lens data")
	fs.BoolVar(&cfg.preserveExif, "preserve-exif", false, "Preserve source JPEG EXIF metadata in output")
	fs.Float64Var(&cfg.frameRatio, "frame-ratio", defaultFrameRatio, "Frame width as a fraction of the shorter image side")
	fs.IntVar(&cfg.quality, "quality", defaultJPEGQuality, "JPEG quality (1-100)")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	cfg.inputPaths = paths
	return cfg, nil
}

func (cfg *Config) validate() error {
	if len(cfg.inputPaths) == 0 {
		return errors.New("provide at least one input path with -f")
	}
	if cfg.frameRatio < 0 || cfg.frameRatio > maxFrameRatio {
		return fmt.Errorf("-frame-ratio must be between 0 and %.2f", maxFrameRatio)
	}
	if cfg.quality < 1 || cfg.quality > 100 {
		return errors.New("-quality must be between 1 and 100")
	}
	if cfg.outputPath != "" && len(cfg.inputPaths) != 1 {
		return errors.New("-o can only be used with one input; use -out-dir for multiple inputs")
	}
	for _, path := range cfg.inputPaths {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("input file %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("input path %q must be a regular file", path)
		}
	}
	return nil
}

func (cfg Config) outputPaths() ([]string, error) {
	if cfg.outputPath != "" {
		return []string{cfg.outputPath}, nil
	}
	outputs := make([]string, len(cfg.inputPaths))
	seen := map[string]string{}
	for i, input := range cfg.inputPaths {
		name := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
		output := filepath.Join(cfg.outputDir, "exiframe-"+name+".jpg")
		key := canonicalPath(output)
		if previous, exists := seen[key]; exists {
			return nil, fmt.Errorf("inputs %q and %q would use the same output path %q", previous, input, output)
		}
		seen[key] = input
		outputs[i] = output
	}
	return outputs, nil
}

func samePath(first, second string) bool {
	if canonicalPath(first) == canonicalPath(second) {
		return true
	}
	firstInfo, err1 := os.Stat(first)
	secondInfo, err2 := os.Stat(second)
	return err1 == nil && err2 == nil && os.SameFile(firstInfo, secondInfo)
}

func canonicalPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(resolved)
	}
	resolvedDir, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err == nil {
		return filepath.Join(resolvedDir, filepath.Base(abs))
	}
	return filepath.Clean(abs)
}

func run(args []string) error {
	cfg, err := parseConfig(args, os.Stderr)
	if err != nil {
		return err
	}
	if err := cfg.validate(); err != nil {
		return err
	}
	if cfg.outputPath == "" {
		if err := os.MkdirAll(cfg.outputDir, 0o755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}
	outputs, err := cfg.outputPaths()
	if err != nil {
		return err
	}
	for i := range cfg.inputPaths {
		for _, otherInput := range cfg.inputPaths {
			if samePath(outputs[i], otherInput) {
				return fmt.Errorf("output %q would overwrite input %q", outputs[i], otherInput)
			}
		}
	}
	var failures []error
	for i, input := range cfg.inputPaths {
		if err := processInput(cfg, input, outputs[i]); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", input, err))
			fmt.Fprintf(os.Stderr, "error: %v\n", failures[len(failures)-1])
			continue
		}
		fmt.Fprintf(os.Stderr, "created %s\n", outputs[i])
	}
	return errors.Join(failures...)
}

func processInput(cfg Config, inputPath, outputPath string) error {
	meta, warnings, err := getExif(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v; continuing without EXIF data\n", err)
	} else {
		for _, warning := range warnings {
			fmt.Fprintf(os.Stderr, "warning: %v\n", warning)
		}
	}
	var exifSegment []byte
	if cfg.preserveExif {
		exifSegment, err = extractExifSegment(inputPath)
		if errors.Is(err, errNoExifSegment) {
			fmt.Fprintf(os.Stderr, "warning: %q has no JPEG EXIF segment; output will not contain EXIF metadata\n", inputPath)
			err = nil
		}
		if err != nil {
			return err
		}
	}
	return renderImage(cfg, inputPath, outputPath, meta, exifSegment)
}
