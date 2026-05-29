package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "aik-airgap-pack",
		Short: "Package AIP components for air-gapped deployment",
		Long:  "Collects all AIP component images, Helm charts, CRDs, and PDP bundle templates into an offline tar.gz bundle.",
		RunE:  runPack,
	}

	rootCmd.Flags().StringP("output", "o", "aip-airgap-bundle.tar.gz", "Output bundle file path")
	rootCmd.Flags().String("helm-chart", "", "Path to Helm chart directory")
	rootCmd.Flags().String("crd-path", "", "Path to CRD YAML directory")
	rootCmd.Flags().String("bundle-template", "", "Path to PDP bundle default template")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runPack(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")
	helmChart, _ := cmd.Flags().GetString("helm-chart")
	crdPath, _ := cmd.Flags().GetString("crd-path")
	bundleTemplate, _ := cmd.Flags().GetString("bundle-template")

	packager := NewPackager(&RealDockerClient{})
	manifest := packager.DefaultManifest()

	config := BundleConfig{
		OutputPath:         output,
		Images:             packager.CollectImages(manifest),
		HelmChartPath:      helmChart,
		CRDPath:            crdPath,
		BundleTemplatePath: bundleTemplate,
	}

	bundlePath, err := packager.CreateBundle(context.Background(), config)
	if err != nil {
		return fmt.Errorf("failed to create bundle: %w", err)
	}

	fmt.Printf("Bundle created: %s\n", bundlePath)
	return nil
}
