// Package main is the entrypoint for the Telekube setup wizard.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// setupConfig mirrors the YAML structure we write for the user.
type setupConfig struct {
	Telegram struct {
		Token    string  `yaml:"token"`
		AdminIDs []int64 `yaml:"admin_ids"`
	} `yaml:"telegram"`
	Clusters []struct {
		Name       string `yaml:"name"`
		Kubeconfig string `yaml:"kubeconfig,omitempty"`
		InCluster  bool   `yaml:"in_cluster,omitempty"`
		Default    bool   `yaml:"default,omitempty"`
	} `yaml:"clusters"`
	Storage struct {
		Backend string `yaml:"backend"`
		SQLite  struct {
			Path string `yaml:"path,omitempty"`
		} `yaml:"sqlite,omitempty"`
	} `yaml:"storage"`
	Modules struct {
		Kubernetes struct {
			Enabled bool `yaml:"enabled"`
		} `yaml:"kubernetes"`
		Watcher struct {
			Enabled bool `yaml:"enabled"`
		} `yaml:"watcher"`
		ArgoCD struct {
			Enabled bool `yaml:"enabled"`
		} `yaml:"argocd"`
	} `yaml:"modules"`
	Server struct {
		Port int `yaml:"port"`
	} `yaml:"server"`
	Log struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"`
	} `yaml:"log"`
	RBAC struct {
		DefaultRole string `yaml:"default_role"`
	} `yaml:"rbac"`
}

// runSetup runs the interactive setup wizard.
// It is called when the binary is invoked with the "setup" argument.
func runSetup(outputPath string) error {
	reader := bufio.NewReader(os.Stdin)

	printBanner()

	cfg := setupConfig{}
	cfg.Server.Port = 8080
	cfg.Log.Level = "info"
	cfg.Log.Format = "console"
	cfg.RBAC.DefaultRole = "viewer"
	cfg.Modules.Kubernetes.Enabled = true
	cfg.Modules.Watcher.Enabled = true
	cfg.Storage.SQLite.Path = "telekube.db"

	// Step 1: Telegram Bot Token
	fmt.Println("\nStep 1/5: Telegram Bot Token")
	fmt.Println("  Enter your bot token from @BotFather:")
	token, err := prompt(reader, "  > ")
	if err != nil {
		return err
	}
	if token == "" {
		return errors.New("bot token is required")
	}
	cfg.Telegram.Token = token

	// Step 2: Admin Telegram IDs
	fmt.Println("\nStep 2/5: Admin Telegram IDs")
	fmt.Println("  Enter admin user IDs (comma-separated):")
	adminIDsStr, err := prompt(reader, "  > ")
	if err != nil {
		return err
	}
	for _, idStr := range strings.Split(adminIDsStr, ",") {
		idStr = strings.TrimSpace(idStr)
		if idStr == "" {
			continue
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			fmt.Printf("  [WARN] Skipping invalid ID: %s\n", idStr)
			continue
		}
		cfg.Telegram.AdminIDs = append(cfg.Telegram.AdminIDs, id)
	}
	if len(cfg.Telegram.AdminIDs) == 0 {
		return errors.New("at least one admin ID is required")
	}

	// Step 3: Kubernetes Clusters
	fmt.Println("\nStep 3/5: Kubernetes Clusters")
	firstCluster := true
	for {
		if firstCluster {
			fmt.Print("  Add a cluster? (y/n): ")
		} else {
			fmt.Print("  Add another cluster? (y/n): ")
		}

		addCluster, err := prompt(reader, "")
		if err != nil {
			return err
		}
		if strings.ToLower(strings.TrimSpace(addCluster)) != "y" {
			break
		}

		clusterName, err := promptWithLabel(reader, "  Cluster name", "default")
		if err != nil {
			return err
		}

		kubeconfigPath, err := promptWithLabel(reader, "  Kubeconfig path (empty for in-cluster)", "")
		if err != nil {
			return err
		}

		isDefault := firstCluster
		if !firstCluster {
			defaultResp, err := promptWithLabel(reader, "  Set as default? (y/n)", "n")
			if err != nil {
				return err
			}
			isDefault = strings.ToLower(strings.TrimSpace(defaultResp)) == "y"
		}

		cluster := struct {
			Name       string `yaml:"name"`
			Kubeconfig string `yaml:"kubeconfig,omitempty"`
			InCluster  bool   `yaml:"in_cluster,omitempty"`
			Default    bool   `yaml:"default,omitempty"`
		}{
			Name:      clusterName,
			InCluster: kubeconfigPath == "",
			Default:   isDefault,
		}
		if kubeconfigPath != "" {
			cluster.Kubeconfig = kubeconfigPath
		}

		cfg.Clusters = append(cfg.Clusters, cluster)
		firstCluster = false
	}

	if len(cfg.Clusters) == 0 {
		// Add a default in-cluster entry
		cfg.Clusters = append(cfg.Clusters, struct {
			Name       string `yaml:"name"`
			Kubeconfig string `yaml:"kubeconfig,omitempty"`
			InCluster  bool   `yaml:"in_cluster,omitempty"`
			Default    bool   `yaml:"default,omitempty"`
		}{
			Name:      "default",
			InCluster: true,
			Default:   true,
		})
		fmt.Println("  (Using in-cluster config as default)")
	}

	// Step 4: Storage Backend
	fmt.Println("\nStep 4/5: Storage Backend")
	fmt.Println("  Select storage:")
	fmt.Println("  1. SQLite (embedded, zero dependencies)")
	fmt.Println("  2. PostgreSQL + Redis (production-grade)")
	storageChoice, err := prompt(reader, "  > ")
	if err != nil {
		return err
	}

	switch strings.TrimSpace(storageChoice) {
	case "2":
		cfg.Storage.Backend = "postgres"
		fmt.Println("  [INFO] Set PG_DSN and REDIS_ADDR environment variables before starting.")
	default:
		cfg.Storage.Backend = "sqlite"
		sqlitePath, err := promptWithLabel(reader, "  SQLite file path", "telekube.db")
		if err != nil {
			return err
		}
		cfg.Storage.SQLite.Path = sqlitePath
	}

	// Step 5: Review & Save
	fmt.Println("\nStep 5/5: Review Configuration")
	printReview(cfg)

	fmt.Print("\n  Save configuration? (y/n): ")
	saveResp, err := prompt(reader, "")
	if err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(saveResp)) != "y" {
		fmt.Println("  Setup cancelled.")
		return nil
	}

	// Ensure directory exists
	if err := os.MkdirAll(dirOf(outputPath), 0750); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0600); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	fmt.Printf("  [OK] Configuration saved to %s\n\n", outputPath)
	fmt.Println("  Start Telekube with:")
	fmt.Println("    $ telekube serve --config " + outputPath)
	fmt.Println()
	return nil
}

func printBanner() {
	fmt.Println()
	fmt.Println("Telekube Setup")
	fmt.Println("==================")
}

func printReview(cfg setupConfig) {
	tokenPreview := cfg.Telegram.Token
	if len(tokenPreview) > 12 {
		tokenPreview = tokenPreview[:6] + "..." + tokenPreview[len(tokenPreview)-4:]
	}

	adminIDs := make([]string, len(cfg.Telegram.AdminIDs))
	for i, id := range cfg.Telegram.AdminIDs {
		adminIDs[i] = strconv.FormatInt(id, 10)
	}

	fmt.Printf("\n  Telegram Token: %s (redacted)\n", tokenPreview)
	fmt.Printf("  Admin IDs:      %s\n", strings.Join(adminIDs, ", "))
	fmt.Println("  Clusters:")
	for _, c := range cfg.Clusters {
		defaultStr := ""
		if c.Default {
			defaultStr = " (default)"
		}
		if c.InCluster {
			fmt.Printf("    - %s%s — in-cluster\n", c.Name, defaultStr)
		} else {
			fmt.Printf("    - %s%s — %s\n", c.Name, defaultStr, c.Kubeconfig)
		}
	}

	fmt.Printf("  Storage: %s\n", cfg.Storage.Backend)

	k8sStatus := "yes"
	if !cfg.Modules.Kubernetes.Enabled {
		k8sStatus = "no"
	}
	argoCDStatus := "no"
	if cfg.Modules.ArgoCD.Enabled {
		argoCDStatus = "yes"
	}
	watcherStatus := "yes"
	if !cfg.Modules.Watcher.Enabled {
		watcherStatus = "no"
	}
	fmt.Printf("  Modules: kubernetes %s, argocd %s, watcher %s\n",
		k8sStatus, argoCDStatus, watcherStatus)
}

func prompt(reader *bufio.Reader, label string) (string, error) {
	if label != "" {
		fmt.Print(label)
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func promptWithLabel(reader *bufio.Reader, label, defaultVal string) (string, error) {
	if defaultVal != "" {
		fmt.Printf("  %s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("  %s: ", label)
	}
	val, err := prompt(reader, "")
	if err != nil {
		return "", err
	}
	val = strings.TrimSpace(val)
	if val == "" {
		return defaultVal, nil
	}
	return val, nil
}

func dirOf(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return "."
	}
	return path[:idx]
}
