/*
latest-catalog-image - Fetch the latest Konflux catalog image information

This tool queries Konflux container registries to find the latest catalog builds,
sorted by build date. It supports multiple tenants, catalogs, and output formats.

USAGE:

	latest-catalog-image [options]

OPTIONS:

	--tenant TENANT      Konflux tenant namespace (default: ocp-bpfman-tenant)
	--catalog CATALOG    Catalog name (default: catalog-ystream)
	                     Examples: catalog-ystream, catalog-zstream, netobserv-catalog
	--registry REGISTRY  Container registry (default: quay.io/redhat-user-workloads)
	--image URL          Full image URL (overrides tenant/catalog/registry)
	--output FORMAT      Output format: text or json (default: text)
	--list N             List last N builds sorted by date (default: show latest only)
	--info               Show detailed metadata (text mode only)
	--timeout DURATION   Timeout for network operations (default: 30s)
	                     Examples: 30s, 5m, 1h30m

EXAMPLES:

Basic usage:

	# Get latest catalog image reference
	./latest-catalog-image

	# Get latest with 5 minute timeout
	./latest-catalog-image --timeout 5m

	# Show detailed information
	./latest-catalog-image --info

	# List last 5 builds
	./latest-catalog-image --list 5

Different catalogs and tenants:

	# BPFMan z-stream (when available)
	./latest-catalog-image --catalog catalog-zstream

	# NetObserv catalog
	./latest-catalog-image --tenant netobserv-tenant --catalog netobserv-catalog

	# Full image URL
	./latest-catalog-image --image quay.io/redhat-user-workloads/ocp-bpfman-tenant/catalog-ystream

JSON output with jq:

	# Get latest as JSON
	./latest-catalog-image --output json

	# Extract just the digest
	./latest-catalog-image --output json | jq -r '.digest'

	# Get full image reference
	./latest-catalog-image --output json | jq -r '"\(.image)@\(.digest)"'

	# Get build date
	./latest-catalog-image --output json | jq -r '.build_date'

	# List mode - get all digests
	./latest-catalog-image --output json --list 5 | jq -r '.images[].digest'

	# Get newest build date from list
	./latest-catalog-image --output json --list 10 | jq -r '.images[0].build_date'

	# Filter by version
	./latest-catalog-image --output json --list 20 | jq '.images[] | select(.version == "4.20.0")'

	# Format as table
	./latest-catalog-image --output json --list 5 | \
	    jq -r '.images[] | [.build_date, .tag[0:7], .version] | @tsv'

Integration examples:

	# Use with just
	just install $(./latest-catalog-image | awk '{print $1}')

	# Use in a script with error checking
	if IMAGE=$(./latest-catalog-image --timeout 10s); then
	    echo "Latest image: ${IMAGE}"
	    just install $(echo "${IMAGE}" | awk '{print $1}')
	else
	    echo "Failed to fetch latest image"
	    exit 1
	fi

	# Check if a newer build exists
	CURRENT="sha256:abc123..."
	LATEST=$(./latest-catalog-image --output json | jq -r '.digest')
	if [ "${CURRENT}" != "${LATEST}" ]; then
	    echo "New build available: ${LATEST}"
	fi

EXIT CODES:

	0   Success
	1   General error
	124 Operation timed out
	130 Interrupted by signal (SIGINT/SIGTERM)

SIGNAL HANDLING:

	The tool handles SIGINT and SIGTERM gracefully, cancelling ongoing
	operations and cleaning up resources before exiting.
*/
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
)

const (
	defaultRegistry = "quay.io/redhat-user-workloads"
	defaultTenant   = "ocp-bpfman-tenant"
	defaultCatalog  = "catalog-ystream"
	maxConcurrency  = 10
)

// Domain types - parse, don't validate

type ImageRef struct {
	Registry string
	Tenant   string
	Catalog  string
}

func (r ImageRef) String() string {
	return fmt.Sprintf("%s/%s/%s", r.Registry, r.Tenant, r.Catalog)
}

func (r ImageRef) Validate() error {
	if r.Registry == "" {
		return fmt.Errorf("registry cannot be empty")
	}
	if r.Tenant == "" {
		return fmt.Errorf("tenant cannot be empty")
	}
	if r.Catalog == "" {
		return fmt.Errorf("catalog cannot be empty")
	}
	return nil
}

type GitCommitTag string

func parseGitCommitTag(s string) (GitCommitTag, error) {
	if len(s) != 40 {
		return "", fmt.Errorf("invalid git commit length: %d", len(s))
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return "", fmt.Errorf("invalid git commit character: %c", c)
		}
	}
	return GitCommitTag(s), nil
}

type ImageMetadata struct {
	Tag       GitCommitTag  `json:"tag"`
	Digest    digest.Digest `json:"digest"`
	BuildDate string        `json:"build_date"`
	Version   string        `json:"version"`
	Created   time.Time     `json:"created"`
}

// JSON output structures
type JSONOutput struct {
	Registry string          `json:"registry"`
	Tenant   string          `json:"tenant"`
	Catalog  string          `json:"catalog"`
	Images   []ImageMetadata `json:"images"`
}

type JSONSingleOutput struct {
	Registry  string `json:"registry"`
	Tenant    string `json:"tenant"`
	Catalog   string `json:"catalog"`
	Image     string `json:"image"`
	Digest    string `json:"digest"`
	BuildDate string `json:"build_date"`
	Version   string `json:"version"`
	Tag       string `json:"tag"`
	Created   string `json:"created"`
}

type Config struct {
	ShowInfo     bool
	ListN        int
	Timeout      time.Duration
	OutputFormat string
	ImageRef     ImageRef
}

type Result struct {
	Images []ImageMetadata
	Error  error
}

// Pure functions - no I/O

func filterGitCommitTags(tags []string) ([]GitCommitTag, error) {
	var commitTags []GitCommitTag
	var parseErrors []string

	for _, tag := range tags {
		if commitTag, err := parseGitCommitTag(tag); err == nil {
			commitTags = append(commitTags, commitTag)
		} else {
			// Silently skip non-commit tags, but track for debugging
			parseErrors = append(parseErrors, tag)
		}
	}

	if len(commitTags) == 0 {
		return nil, fmt.Errorf("no valid git commit tags found among %d tags", len(tags))
	}

	return commitTags, nil
}

func sortByBuildDate(images []ImageMetadata) []ImageMetadata {
	sorted := make([]ImageMetadata, len(images))
	copy(sorted, images)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].BuildDate > sorted[j].BuildDate
	})
	return sorted
}

func formatImageReference(imageRef ImageRef, metadata ImageMetadata) string {
	return fmt.Sprintf("%s@%s %s", imageRef.String(), metadata.Digest, metadata.BuildDate)
}

func formatDetailedInfo(imageRef ImageRef, metadata ImageMetadata) string {
	var b strings.Builder
	b.WriteString("Image Reference:\n")
	b.WriteString(fmt.Sprintf("  %s@%s\n\n", imageRef.String(), metadata.Digest))
	b.WriteString(fmt.Sprintf("Latest Tag: %s\n\n", metadata.Tag))
	b.WriteString("Build Information:\n")
	b.WriteString(fmt.Sprintf("  Tenant: %s\n", imageRef.Tenant))
	b.WriteString(fmt.Sprintf("  Catalog: %s\n", imageRef.Catalog))
	b.WriteString(fmt.Sprintf("  Build Date: %s\n", metadata.BuildDate))
	b.WriteString(fmt.Sprintf("  Version: %s\n", metadata.Version))
	b.WriteString(fmt.Sprintf("  Git Commit: %s\n", metadata.Tag))
	b.WriteString(fmt.Sprintf("Created: %s\n", metadata.Created.Format(time.RFC3339Nano)))
	b.WriteString("Architecture: amd64\n")
	return b.String()
}

func formatList(imageRef ImageRef, images []ImageMetadata, n int) string {
	var b strings.Builder
	limit := n
	if limit > len(images) {
		limit = len(images)
	}

	b.WriteString(fmt.Sprintf("Last %d %s builds (sorted by build date, newest first):\n\n", limit, imageRef.Catalog))
	for i := 0; i < limit; i++ {
		b.WriteString(formatImageReference(imageRef, images[i]))
		b.WriteString("\n")
	}
	return b.String()
}

func formatJSON(imageRef ImageRef, images []ImageMetadata, singleItem bool) (string, error) {
	if singleItem && len(images) > 0 {
		// Single item output (default mode)
		output := JSONSingleOutput{
			Registry:  imageRef.Registry,
			Tenant:    imageRef.Tenant,
			Catalog:   imageRef.Catalog,
			Image:     imageRef.String(),
			Digest:    string(images[0].Digest),
			BuildDate: images[0].BuildDate,
			Version:   images[0].Version,
			Tag:       string(images[0].Tag),
			Created:   images[0].Created.Format(time.RFC3339Nano),
		}
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	// List output
	output := JSONOutput{
		Registry: imageRef.Registry,
		Tenant:   imageRef.Tenant,
		Catalog:  imageRef.Catalog,
		Images:   images,
	}
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// I/O functions - interact with external systems

func fetchTags(ctx context.Context, imageRef ImageRef) ([]string, error) {
	ref, err := docker.ParseReference(fmt.Sprintf("//%s", imageRef.String()))
	if err != nil {
		return nil, fmt.Errorf("parsing reference %s: %w", imageRef, err)
	}

	sys := &types.SystemContext{
		OSChoice:           "linux",
		ArchitectureChoice: "amd64",
	}

	tags, err := docker.GetRepositoryTags(ctx, sys, ref)
	if err != nil {
		return nil, fmt.Errorf("fetching tags for %s: %w", imageRef, err)
	}

	return tags, nil
}

func fetchImageMetadata(ctx context.Context, imageRef ImageRef, tag GitCommitTag) (ImageMetadata, error) {
	taggedRef := fmt.Sprintf("%s:%s", imageRef.String(), tag)
	ref, err := docker.ParseReference(fmt.Sprintf("//%s", taggedRef))
	if err != nil {
		return ImageMetadata{}, fmt.Errorf("parsing reference %s: %w", taggedRef, err)
	}

	sys := &types.SystemContext{
		OSChoice:           "linux",
		ArchitectureChoice: "amd64",
		// Allow manifest lists/indexes
		DockerArchiveAdditionalTags: nil,
	}

	// Use NewImage which handles manifest lists properly
	img, err := ref.NewImage(ctx, sys)
	if err != nil {
		return ImageMetadata{}, fmt.Errorf("creating image for %s: %w", taggedRef, err)
	}
	defer img.Close()

	// Get the manifest digest
	manifestBlob, _, err := img.Manifest(ctx)
	if err != nil {
		return ImageMetadata{}, fmt.Errorf("fetching manifest for %s: %w", taggedRef, err)
	}

	manifestDigest, err := manifest.Digest(manifestBlob)
	if err != nil {
		return ImageMetadata{}, fmt.Errorf("computing digest for %s: %w", taggedRef, err)
	}

	// Get inspection data
	inspect, err := img.Inspect(ctx)
	if err != nil {
		return ImageMetadata{}, fmt.Errorf("inspecting image %s: %w", taggedRef, err)
	}

	metadata := ImageMetadata{
		Tag:    tag,
		Digest: manifestDigest,
	}

	if inspect.Labels != nil {
		metadata.BuildDate = inspect.Labels["build-date"]
		metadata.Version = inspect.Labels["version"]
	}

	if inspect.Created != nil {
		metadata.Created = *inspect.Created
	}

	// Fail fast if we couldn't get the metadata
	if metadata.BuildDate == "" {
		return ImageMetadata{}, fmt.Errorf("no build date found for %s", taggedRef)
	}

	return metadata, nil
}

func fetchAllImageMetadata(ctx context.Context, imageRef ImageRef, tags []GitCommitTag) ([]ImageMetadata, error) {
	var wg sync.WaitGroup
	results := make(chan Result, len(tags))
	semaphore := make(chan struct{}, maxConcurrency)

	for _, tag := range tags {
		wg.Add(1)
		go func(tag GitCommitTag) {
			defer wg.Done()

			// Check for cancellation before starting work
			select {
			case <-ctx.Done():
				results <- Result{Error: fmt.Errorf("tag %s: %w", tag, ctx.Err())}
				return
			default:
			}

			// Rate limiting with cancellation support
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				results <- Result{Error: fmt.Errorf("tag %s: %w", tag, ctx.Err())}
				return
			}

			metadata, err := fetchImageMetadata(ctx, imageRef, tag)
			if err != nil {
				results <- Result{Error: fmt.Errorf("tag %s: %w", tag, err)}
				return
			}
			results <- Result{Images: []ImageMetadata{metadata}}
		}(tag)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var images []ImageMetadata
	var errs []error

	for result := range results {
		if result.Error != nil {
			// Only collect non-cancellation errors
			if !errors.Is(result.Error, context.Canceled) {
				errs = append(errs, result.Error)
			}
		} else {
			images = append(images, result.Images...)
		}
	}

	// Check if we were cancelled
	if ctx.Err() != nil {
		return nil, fmt.Errorf("operation cancelled: %w", ctx.Err())
	}

	// Fail if we couldn't fetch any images
	if len(images) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("failed to fetch any images: %v", errors.Join(errs...))
	}

	return images, nil
}

func parseConfig() (Config, error) {
	cfg := Config{
		Timeout: 30 * time.Second, // Default timeout
	}

	var (
		tenant   string
		catalog  string
		registry string
		imageURL string
	)

	flag.BoolVar(&cfg.ShowInfo, "info", false, "Show detailed metadata for latest")
	flag.IntVar(&cfg.ListN, "list", 0, "List last N builds (0 = don't list)")
	flag.DurationVar(&cfg.Timeout, "timeout", 30*time.Second, "Timeout for network operations (e.g. 30s, 5m, 1h)")
	flag.StringVar(&tenant, "tenant", defaultTenant, "Konflux tenant namespace")
	flag.StringVar(&catalog, "catalog", defaultCatalog, "Catalog name (e.g. catalog-ystream, catalog-zstream, netobserv-catalog)")
	flag.StringVar(&registry, "registry", defaultRegistry, "Container registry")
	flag.StringVar(&imageURL, "image", "", "Full image URL (overrides tenant/catalog/registry)")
	flag.StringVar(&cfg.OutputFormat, "output", "text", "Output format: text or json")
	flag.Parse()

	// Build ImageRef from components or parse from URL
	if imageURL != "" {
		// Parse the provided image URL
		parts := strings.Split(strings.TrimPrefix(imageURL, "docker://"), "/")
		if len(parts) < 3 {
			return Config{}, fmt.Errorf("invalid image URL format: %s (expected registry/tenant/catalog)", imageURL)
		}

		// Handle quay.io/redhat-user-workloads/tenant/catalog format
		if len(parts) == 4 && parts[0] == "quay.io" && parts[1] == "redhat-user-workloads" {
			cfg.ImageRef = ImageRef{
				Registry: fmt.Sprintf("%s/%s", parts[0], parts[1]),
				Tenant:   parts[2],
				Catalog:  parts[3],
			}
		} else if len(parts) == 3 {
			cfg.ImageRef = ImageRef{
				Registry: parts[0],
				Tenant:   parts[1],
				Catalog:  parts[2],
			}
		} else {
			return Config{}, fmt.Errorf("unsupported image URL format: %s", imageURL)
		}
	} else {
		cfg.ImageRef = ImageRef{
			Registry: registry,
			Tenant:   tenant,
			Catalog:  catalog,
		}
	}

	// Validate ImageRef
	if err := cfg.ImageRef.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid image configuration: %w", err)
	}

	// Validate other config
	if cfg.ListN < 0 {
		return Config{}, fmt.Errorf("list count cannot be negative: %d", cfg.ListN)
	}

	if cfg.Timeout <= 0 {
		return Config{}, fmt.Errorf("timeout must be positive: %s", cfg.Timeout)
	}

	if cfg.OutputFormat != "text" && cfg.OutputFormat != "json" {
		return Config{}, fmt.Errorf("output format must be 'text' or 'json': %s", cfg.OutputFormat)
	}

	return cfg, nil
}

// Main orchestrator
func run(ctx context.Context, cfg Config) error {
	// Fetch all tags
	tags, err := fetchTags(ctx, cfg.ImageRef)
	if err != nil {
		return fmt.Errorf("fetching tags: %w", err)
	}

	// Filter for git commit tags
	commitTags, err := filterGitCommitTags(tags)
	if err != nil {
		return fmt.Errorf("filtering tags: %w", err)
	}

	// Fetch metadata for all commit tags
	images, err := fetchAllImageMetadata(ctx, cfg.ImageRef, commitTags)
	if err != nil {
		return fmt.Errorf("fetching metadata: %w", err)
	}

	if len(images) == 0 {
		return errors.New("no images with metadata found")
	}

	// Sort by build date
	sorted := sortByBuildDate(images)

	// Output based on format and mode
	if cfg.OutputFormat == "json" {
		var jsonStr string
		var err error

		if cfg.ListN > 0 {
			// List mode - return multiple items
			limit := cfg.ListN
			if limit > len(sorted) {
				limit = len(sorted)
			}
			jsonStr, err = formatJSON(cfg.ImageRef, sorted[:limit], false)
		} else {
			// Default or info mode - return single item
			jsonStr, err = formatJSON(cfg.ImageRef, sorted[:1], true)
		}

		if err != nil {
			return fmt.Errorf("formatting JSON output: %w", err)
		}
		fmt.Println(jsonStr)
	} else {
		// Text output
		switch {
		case cfg.ListN > 0:
			fmt.Print(formatList(cfg.ImageRef, sorted, cfg.ListN))
		case cfg.ShowInfo:
			fmt.Print(formatDetailedInfo(cfg.ImageRef, sorted[0]))
		default:
			fmt.Println(formatImageReference(cfg.ImageRef, sorted[0]))
		}
	}

	return nil
}

func main() {
	cfg, err := parseConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Set up context with timeout and signal handling
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// Handle SIGTERM and SIGINT
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run in a goroutine to allow signal handling
	errChan := make(chan error, 1)
	go func() {
		errChan <- run(ctx, cfg)
	}()

	// Wait for either completion or signal
	select {
	case sig := <-sigChan:
		// Graceful shutdown on signal
		fmt.Fprintf(os.Stderr, "\nReceived signal %s, shutting down gracefully...\n", sig)
		cancel()

		// Give the operation a chance to clean up (max 5 seconds)
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		select {
		case err := <-errChan:
			if err != nil && !errors.Is(err, context.Canceled) {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		case <-cleanupCtx.Done():
			fmt.Fprintf(os.Stderr, "Shutdown timeout exceeded\n")
			os.Exit(130) // Standard exit code for SIGINT
		}
		os.Exit(130) // Standard exit code for terminated by signal

	case err := <-errChan:
		// Normal completion
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				fmt.Fprintf(os.Stderr, "Error: operation timed out after %s\n", cfg.Timeout)
				os.Exit(124) // Standard timeout exit code
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}
