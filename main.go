// TerraViz: one binary for cross-state Terraform dependency analysis and
// interactive resource-level architecture diagrams.
//
//	terraviz [-min 0.5] [-serve] [-query SQL] [name=]state.tfstate ...
//	terraviz scan terraform state.tfstate
//	terraviz scan aws [--regions us-east-1] [--profile default]
//	terraviz compare a.json b.json
//	terraviz export diagram.json --format svg
//	terraviz serve [diagram.json]
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"terraviz/api"
	"terraviz/db"
	"terraviz/internal/exporters"
	"terraviz/internal/models"
	"terraviz/internal/parsers"
	"terraviz/internal/scanners"
	"terraviz/internal/server"
	"terraviz/legacyweb"
	"terraviz/resolver"
	"terraviz/state"
)

// Build-time variables, set via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const (
	defaultOutputFile = "diagram.json"
	defaultHost       = "localhost"
	defaultPort       = 8080
	defaultRegion     = "us-east-1"
	filePermissions   = 0o644
)

var (
	titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true).Padding(1, 2)
	infoStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "terraviz: "+format+"\n", args...)
	os.Exit(1)
}

func printHeader() { fmt.Println(titleStyle.Render("TerraViz " + version)) }

func parseArg(a string) (name, path string) {
	if i := strings.Index(a, "="); i > 0 {
		return a[:i], a[i+1:]
	}
	abs, err := filepath.Abs(a)
	if err != nil {
		abs = a
	}
	base := filepath.Base(abs)
	if base == "terraform.tfstate" {
		return filepath.Base(filepath.Dir(abs)), a
	}
	return strings.TrimSuffix(strings.TrimSuffix(base, ".json"), ".tfstate"), a
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	return os.WriteFile(path, data, filePermissions)
}

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

type dashboardOptions struct {
	min     float64
	verbose bool
	jsonOut string
	serve   bool
	addr    string
	dbPath  string
	query   string
}

func runDashboard(cmd *cobra.Command, args []string) error {
	o := dashboardOptions{
		min:     mustFloat(cmd.Flags(), "min"),
		verbose: mustBool(cmd.Flags(), "v"),
		jsonOut: mustString(cmd.Flags(), "json"),
		serve:   mustBool(cmd.Flags(), "serve"),
		addr:    mustString(cmd.Flags(), "addr"),
		dbPath:  mustString(cmd.Flags(), "db"),
		query:   mustString(cmd.Flags(), "query"),
	}
	return executeDashboard(o, args)
}

func mustFloat(f *pflag.FlagSet, name string) float64 {
	v, _ := f.GetFloat64(name)
	return v
}
func mustBool(f *pflag.FlagSet, name string) bool {
	v, _ := f.GetBool(name)
	return v
}
func mustString(f *pflag.FlagSet, name string) string {
	v, _ := f.GetString(name)
	return v
}

func executeDashboard(o dashboardOptions, args []string) error {
	min, verbose, jsonOut := o.min, o.verbose, o.jsonOut
	serve, addr, dbPath, query := o.serve, o.addr, o.dbPath, o.query

	if serve {
		if dbPath == "" {
			return errors.New("-serve needs a -db path")
		}
		return runServer(addr, dbPath, "")
	}
	if query != "" {
		if dbPath == "" {
			return errors.New("-query needs a -db path")
		}
		d, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer d.Close()
		res, err := d.Query(query)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	if len(args) < 2 {
		return errors.New("need at least two statefiles, or use -serve / a subcommand; see --help")
	}

	srcs := make([]resolver.Source, 0, len(args))
	seen := map[string]bool{}
	for _, a := range args {
		name, path := parseArg(a)
		if seen[name] {
			return fmt.Errorf("duplicate state name %q; use name=path to disambiguate", name)
		}
		seen[name] = true
		f, err := state.Load(path)
		if err != nil {
			return err
		}
		srcs = append(srcs, resolver.Source{Name: name, Filename: path, File: f})
	}
	edges := resolver.Resolve(srcs)
	deps := resolver.Summarize(edges, min)

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "FROM\tTO\tCONF\tEVIDENCE\tKINDS")
	for _, d := range deps {
		fmt.Fprintf(tw, "%s\t%s\t%.2f\t%d\t%s\n", d.From, d.To, d.Confidence, d.Evidence, strings.Join(d.Kinds, ","))
	}
	tw.Flush()
	if verbose {
		fmt.Println()
		tw = tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "FROM\tFROM ADDR\tATTR\tTO\tTO ADDR\tKIND\tCONF\tMATCHED")
		for _, e := range edges {
			if e.Confidence < min {
				continue
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%.2f\t%s\n",
				e.From, e.FromAddr, e.FromAttr, e.To, e.ToAddr, e.Kind, e.Confidence, e.Matched)
		}
		tw.Flush()
	}
	if jsonOut != "" {
		b, _ := json.MarshalIndent(map[string]any{"dependencies": deps, "edges": edges}, "", "  ")
		if err := os.WriteFile(jsonOut, b, filePermissions); err != nil {
			return err
		}
	}
	if dbPath != "" {
		d, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer d.Close()
		if err := d.Ingest(srcs); err != nil {
			return err
		}
	}
	return nil
}

func runServer(addr, dbPath, diagramFile string) error {
	d, err := db.Open(dbPath)
	if err != nil {
		return err
	}
	defer d.Close()

	legacy := server.NewServer(defaultHost, defaultPort)
	if diagramFile != "" {
		if err := legacy.LoadDiagram(diagramFile); err != nil {
			return err
		}
	}
	apiServer := api.New(d)
	mux := apiServer.Mux()
	mux.Handle("/terraviz/", http.StripPrefix("/terraviz", legacy.Handler()))

	// Root-level /icons/ serves the full embedded AWS pack plus the small
	// provider/service icons used by the dashboard.
	iconsFS, err := fs.Sub(legacyweb.EmbeddedFiles, "icons")
	if err != nil {
		return err
	}
	mux.Handle("/icons/", http.StripPrefix("/icons", http.FileServer(http.FS(iconsFS))))

	srv := &http.Server{
		Addr:              addr,
		Handler:           apiServer.Wrap(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	fmt.Printf("TerraViz listening on %s (db %s)\n", addr, d.Path())
	fmt.Printf("Dashboard: http://%s/  ·  Resource diagram: http://%s/terraviz/\n", addr, addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

var rootCmd = &cobra.Command{
	Use:     "terraviz",
	Short:   "Cross-state Terraform dependencies and interactive architecture diagrams",
	Version: version,
	RunE:    runDashboard,
	Long: `TerraViz ingests Terraform statefiles, resolves cross-state dependencies,
stores them in an embedded SQLite database, and serves a dashboard for SQL,
account-aware architecture diagrams and resource-level exploration.`,
}

var scanCmd = &cobra.Command{Use: "scan", Short: "Scan infrastructure and generate diagrams"}

var terraformCmd = &cobra.Command{
	Use:   "terraform [state-file]",
	Short: "Scan a Terraform state file into a diagram JSON",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		printHeader()
		fmt.Println(infoStyle.Render("Scanning Terraform state: " + args[0]))
		result, err := parsers.NewTerraformParser().ParseStateFile(args[0])
		if err != nil {
			return fmt.Errorf("parsing Terraform state: %w", err)
		}
		out, _ := cmd.Flags().GetString("output")
		if err := writeJSON(out, result); err != nil {
			return err
		}
		printScanSummary(result.Stats, out, result.Errors, result.Warnings)
		return nil
	},
}

var awsCmd = &cobra.Command{
	Use:   "aws",
	Short: "Scan a live AWS account",
	RunE: func(cmd *cobra.Command, _ []string) error {
		printHeader()
		fmt.Println(infoStyle.Render("Scanning live AWS account..."))
		regions, _ := cmd.Flags().GetStringSlice("regions")
		profile, _ := cmd.Flags().GetString("profile")
		out, _ := cmd.Flags().GetString("output")
		ctx := context.Background()
		scanner, err := scanners.NewAWSScanner(ctx, profile, regions)
		if err != nil {
			return fmt.Errorf("creating AWS scanner: %w", err)
		}
		result, err := scanner.ScanAccount(ctx)
		if err != nil {
			return fmt.Errorf("scanning AWS account: %w", err)
		}
		if err := writeJSON(out, result); err != nil {
			return err
		}
		printScanSummary(result.Stats, out, result.Errors, result.Warnings)
		return nil
	},
}

var compareCmd = &cobra.Command{
	Use:   "compare [scan1] [scan2]",
	Short: "Compare two scans to see changes",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		printHeader()
		base, err := loadScanResult(args[0])
		if err != nil {
			return fmt.Errorf("loading base scan: %w", err)
		}
		other, err := loadScanResult(args[1])
		if err != nil {
			return fmt.Errorf("loading compare scan: %w", err)
		}
		result := models.CompareDiagrams(&base.Diagram, &other.Diagram)
		out, _ := cmd.Flags().GetString("output")
		out = strings.ReplaceAll(out, ".json", "_comparison.json")
		if err := writeJSON(out, result); err != nil {
			return err
		}
		fmt.Printf("Comparison: +%d / -%d / ~%d resources, +%d -%d connections\n",
			result.Summary.AddedCount, result.Summary.RemovedCount,
			result.Summary.ModifiedCount, len(result.ConnectionsAdded), len(result.ConnectionsRemoved))
		fmt.Printf("Results saved to: %s\n", out)
		return nil
	},
}

var exportCmd = &cobra.Command{
	Use:   "export [input]",
	Short: "Export a diagram to HTML, PNG, SVG, CSV, Mermaid, DOT, draw.io or Markdown",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		printHeader()
		format, _ := cmd.Flags().GetString("format")
		out, _ := cmd.Flags().GetString("output")
		diagram, err := loadDiagramForExport(args[0])
		if err != nil {
			return err
		}
		data, err := exporters.Export(diagram, format)
		if err != nil {
			return fmt.Errorf("exporting diagram: %w", err)
		}
		if out == "" {
			out = strings.TrimSuffix(args[0], filepath.Ext(args[0])) + exporters.Extension(format)
		}
		if err := os.WriteFile(out, data, filePermissions); err != nil {
			return err
		}
		fmt.Printf("Exported %d resources and %d connections as %s to %s\n",
			len(diagram.Resources), len(diagram.Connections), format, out)
		return nil
	},
}

func loadDiagramForExport(path string) (*models.Diagram, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err == nil {
		if _, ok := envelope["diagram"]; ok {
			var result models.ScanResult
			if err := json.Unmarshal(data, &result); err != nil {
				return nil, fmt.Errorf("parsing %s: %w", path, err)
			}
			return &result.Diagram, nil
		}
		if _, ok := envelope["resources"]; ok {
			var diagram models.Diagram
			if err := json.Unmarshal(data, &diagram); err != nil {
				return nil, fmt.Errorf("parsing %s: %w", path, err)
			}
			return &diagram, nil
		}
	}
	result, err := parsers.NewTerraformParser().ParseStateFile(path)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &result.Diagram, nil
}

var serveCmd = &cobra.Command{
	Use:   "serve [diagram.json]",
	Short: "Serve the dashboard and the resource-level diagram",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		printHeader()
		addr, _ := cmd.Flags().GetString("addr")
		dbPath, _ := cmd.Flags().GetString("db")
		diagram := ""
		if len(args) == 1 {
			diagram = args[0]
		}
		return runServer(addr, dbPath, diagram)
	},
}

func init() {
	scanCmd.AddCommand(terraformCmd, awsCmd)
	rootCmd.AddCommand(scanCmd, compareCmd, exportCmd, serveCmd)

	rootCmd.PersistentFlags().StringP("output", "o", defaultOutputFile, "Output file for scan results")
	rootCmd.PersistentFlags().BoolP("v", "v", false, "Verbose output")
	rootCmd.Flags().Float64("min", 0.5, "Minimum confidence to report")
	rootCmd.Flags().String("json", "", "Write dependencies and edges to JSON")
	rootCmd.Flags().Bool("serve", false, "Run the dashboard server")
	rootCmd.Flags().String("addr", ":8080", "HTTP listen address")
	rootCmd.Flags().String("db", "terraviz.db", "SQLite file; empty disables persistence")
	rootCmd.Flags().String("query", "", "Run a read-only SQL query and print JSON")

	exportCmd.Flags().StringP("format", "f", exporters.FormatHTML,
		"Export format: "+strings.Join(exporters.Formats, "|"))
	exportCmd.Flags().StringP("output", "o", "", "Output file (default: input name with format extension)")
	awsCmd.Flags().StringSliceP("regions", "r", []string{defaultRegion}, "AWS regions to scan")
	awsCmd.Flags().StringP("profile", "p", "", "AWS profile to use")
	serveCmd.Flags().String("addr", ":8080", "HTTP listen address")
	serveCmd.Flags().String("db", "terraviz.db", "SQLite file")

	rootCmd.SetVersionTemplate(fmt.Sprintf("terraviz %s (commit: %s, built: %s)\n", version, commit, date))
}

// runLegacyCLI keeps the original terraquery flag interface working: flags
// are handled by the standard flag package and positional args are statefiles.
func runLegacyCLI(args []string) error {
	fs := flag.NewFlagSet("terraviz", flag.ContinueOnError)
	o := dashboardOptions{}
	fs.Float64Var(&o.min, "min", 0.5, "minimum confidence to report")
	fs.BoolVar(&o.verbose, "v", false, "verbose output")
	fs.StringVar(&o.jsonOut, "json", "", "write dependencies and edges to JSON")
	fs.BoolVar(&o.serve, "serve", false, "run the dashboard server")
	fs.StringVar(&o.addr, "addr", ":8080", "HTTP listen address")
	fs.StringVar(&o.dbPath, "db", "terraviz.db", "SQLite file; empty disables persistence")
	fs.StringVar(&o.query, "query", "", "run a read-only SQL query")
	showVersion := fs.Bool("version", false, "print version")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Printf("terraviz %s (commit: %s, built: %s)\n", version, commit, date)
		return nil
	}
	return executeDashboard(o, fs.Args())
}

func main() {
	args := os.Args[1:]
	isSubcommand := false
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			switch a {
			case "scan", "compare", "export", "serve":
				isSubcommand = true
			}
			break
		}
	}
	if !isSubcommand {
		if err := runLegacyCLI(args); err != nil {
			fatalf("%v", err)
		}
		return
	}
	rootCmd.SetArgs(args)
	if err := rootCmd.Execute(); err != nil {
		fatalf("%v", err)
	}
}
