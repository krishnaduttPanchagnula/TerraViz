package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"terraviz/internal/models"
	"terraviz/internal/parsers"
	"terraviz/internal/scanners"
	"terraviz/internal/server"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// CLI defaults.
const (
	defaultOutputFile = "diagram.json"
	defaultHost       = "localhost"
	defaultPort       = 8080
	defaultRegion     = "us-east-1"
	filePermissions   = 0o644
)

// Build-time variables, set via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true).
			Padding(1, 2)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

// printHeader prints the application banner.
func printHeader() {
	fmt.Println(titleStyle.Render("TerraViz " + version))
}

// writeJSON marshals v as indented JSON and writes it to path.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	if err := os.WriteFile(path, data, filePermissions); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// printScanSummary prints resource/connection counts and any errors/warnings.
func printScanSummary(stats models.ScanStats, outputFile string, errors, warnings []string) {
	fmt.Printf("Successfully parsed %d resources with %d connections\n",
		stats.ResourceCount, stats.ConnectionCount)
	fmt.Printf("Results saved to: %s\n", outputFile)

	if stats.ScanDurationMs > 0 {
		fmt.Printf("Scan took: %dms\n", stats.ScanDurationMs)
	}
	if len(errors) > 0 {
		fmt.Printf("%d errors occurred during processing\n", len(errors))
	}
	if len(warnings) > 0 {
		fmt.Printf("%d warnings occurred during processing\n", len(warnings))
	}
}

// fatalf prints an error message to stderr and exits.
func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// loadScanResult reads a JSON scan result file from disk.
func loadScanResult(path string) (*models.ScanResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var result models.ScanResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &result, nil
}

var rootCmd = &cobra.Command{
	Use:     "terraviz",
	Short:   "Generate interactive architecture diagrams from cloud infrastructure",
	Version: version,
	Long: `Cloud Architecture Visualizer is a tool that automatically generates
interactive architecture diagrams from your live cloud setup or Terraform state files.`,
}

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan infrastructure and generate diagrams",
	Long:  `Scan your cloud infrastructure from various sources and generate interactive diagrams.`,
}

var terraformCmd = &cobra.Command{
	Use:   "terraform [state-file]",
	Short: "Scan Terraform state file",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		printHeader()
		fmt.Println(infoStyle.Render("Scanning Terraform state: " + args[0]))

		parser := parsers.NewTerraformParser()
		result, err := parser.ParseStateFile(args[0])
		if err != nil {
			fatalf("Error parsing Terraform state: %v", err)
		}

		outputFile, _ := cmd.Flags().GetString("output")
		if err := writeJSON(outputFile, result); err != nil {
			fatalf("Error saving result: %v", err)
		}

		printScanSummary(result.Stats, outputFile, result.Errors, result.Warnings)
	},
}

var awsCmd = &cobra.Command{
	Use:   "aws",
	Short: "Scan live AWS account",
	Run: func(cmd *cobra.Command, args []string) {
		printHeader()
		fmt.Println(infoStyle.Render("Scanning live AWS account..."))

		regions, _ := cmd.Flags().GetStringSlice("regions")
		profile, _ := cmd.Flags().GetString("profile")
		outputFile, _ := cmd.Flags().GetString("output")

		ctx := context.Background()

		scanner, err := scanners.NewAWSScanner(ctx, profile, regions)
		if err != nil {
			fatalf("Error creating AWS scanner: %v", err)
		}

		result, err := scanner.ScanAccount(ctx)
		if err != nil {
			fatalf("Error scanning AWS account: %v", err)
		}

		if err := writeJSON(outputFile, result); err != nil {
			fatalf("Error saving result: %v", err)
		}

		printScanSummary(result.Stats, outputFile, result.Errors, result.Warnings)
	},
}

var compareCmd = &cobra.Command{
	Use:   "compare [scan1] [scan2]",
	Short: "Compare two scans to see changes",
	Args:  cobra.ExactArgs(2),
	Run:   runCompare,
}

func runCompare(cmd *cobra.Command, args []string) {
	printHeader()
	fmt.Println(infoStyle.Render("Comparing scans: " + args[0] + " vs " + args[1]))

	baseScan, err := loadScanResult(args[0])
	if err != nil {
		fatalf("Error loading base scan: %v", err)
	}

	compareScan, err := loadScanResult(args[1])
	if err != nil {
		fatalf("Error loading compare scan: %v", err)
	}

	comparison := models.CompareDiagrams(&baseScan.Diagram, &compareScan.Diagram)

	outputFile, _ := cmd.Flags().GetString("output")
	outputFile = strings.ReplaceAll(outputFile, ".json", "_comparison.json")

	if err := writeJSON(outputFile, comparison); err != nil {
		fatalf("Error saving comparison: %v", err)
	}

	printComparisonSummary(comparison, outputFile)

	verbose, _ := cmd.Flags().GetBool("verbose")
	if verbose {
		printComparisonDetails(comparison)
	}
}

func printComparisonSummary(c *models.ComparisonResult, outputFile string) {
	fmt.Printf("Comparison Summary:\n")
	fmt.Printf("  Added:       %d resources\n", c.Summary.AddedCount)
	fmt.Printf("  Removed:     %d resources\n", c.Summary.RemovedCount)
	fmt.Printf("  Modified:    %d resources\n", c.Summary.ModifiedCount)
	fmt.Printf("  Unchanged:   %d resources\n", c.Summary.UnchangedCount)
	fmt.Printf("  Connections: +%d -%d\n", len(c.ConnectionsAdded), len(c.ConnectionsRemoved))
	fmt.Printf("Results saved to: %s\n", outputFile)
}

func printComparisonDetails(c *models.ComparisonResult) {
	fmt.Println("\nDetailed Changes:")

	if len(c.Added) > 0 {
		fmt.Printf("\nAdded Resources (%d):\n", len(c.Added))
		for _, resource := range c.Added {
			fmt.Printf("  + %s (%s)\n", resource.Name, resource.Type)
		}
	}

	if len(c.Removed) > 0 {
		fmt.Printf("\nRemoved Resources (%d):\n", len(c.Removed))
		for _, resource := range c.Removed {
			fmt.Printf("  - %s (%s)\n", resource.Name, resource.Type)
		}
	}

	if len(c.Modified) > 0 {
		fmt.Printf("\nModified Resources (%d):\n", len(c.Modified))
		for _, diff := range c.Modified {
			fmt.Printf("  ~ %s:\n", diff.ResourceID)
			for field, change := range diff.Changes {
				fmt.Printf("    %s: %v -> %v\n", field, change.OldValue, change.NewValue)
			}
		}
	}
}

var serveCmd = &cobra.Command{
	Use:   "serve [diagram-file]",
	Short: "Serve interactive diagram locally",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		printHeader()
		fmt.Println(infoStyle.Render("Starting web server for diagram: " + args[0]))

		port, _ := cmd.Flags().GetInt("port")
		host, _ := cmd.Flags().GetString("host")

		webServer := server.NewServer(host, port)
		if err := webServer.LoadDiagram(args[0]); err != nil {
			fatalf("Error loading diagram: %v", err)
		}

		fmt.Printf("Diagram loaded successfully\n")
		fmt.Printf("Open http://%s:%d in your browser\n", host, port)

		if err := webServer.Start(); err != nil {
			fatalf("Error starting server: %v", err)
		}
	},
}

func init() {
	scanCmd.AddCommand(terraformCmd)
	scanCmd.AddCommand(awsCmd)

	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(compareCmd)
	rootCmd.AddCommand(serveCmd)

	// Global flags
	rootCmd.PersistentFlags().StringP("output", "o", defaultOutputFile, "Output file for the scan results")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")

	// Version template with build metadata.
	rootCmd.SetVersionTemplate(fmt.Sprintf("terraviz %s (commit: %s, built: %s)\n", version, commit, date))

	// AWS-specific flags
	awsCmd.Flags().StringSliceP("regions", "r", []string{defaultRegion}, "AWS regions to scan")
	awsCmd.Flags().StringP("profile", "p", "", "AWS profile to use")

	// Serve-specific flags
	serveCmd.Flags().IntP("port", "P", defaultPort, "Port to serve on")
	serveCmd.Flags().StringP("host", "H", defaultHost, "Host to bind to")
}

func main() {
	ctx := context.Background()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
