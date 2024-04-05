package main

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ryanwilsonperkin/promql/promfiles"
)

func main() {
	var metrics = promfiles.NewMetrics()
	var result promfiles.LoadResult

	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: promql BACKUP_DIR")
		os.Exit(1)
	}

	backupDir := os.Args[1]
	dashboardFiles := listFiles(filepath.Join(backupDir, "dashboards"))
	monitorFiles := listFiles(filepath.Join(backupDir, "monitors"))
	sloFiles := listFiles(filepath.Join(backupDir, "slos"))

	for _, dashboard := range Map(dashboardFiles, promfiles.NewDashboardFile) {
		result.Add(dashboard.Load(&metrics))
	}

	for _, monitor := range Map(monitorFiles, promfiles.NewMonitorFile) {
		result.Add(monitor.Load(&metrics))
	}

	for _, slo := range Map(sloFiles, promfiles.NewSLOFile) {
		result.Add(slo.Load(&metrics))
	}

	for _, metric := range metrics.Entries {
		fmt.Println(metric.Location, metric.Name, strings.Join(metric.Labels, " "))
	}
	fmt.Fprintln(os.Stderr, "Skipped:   ", result.Skipped)
	fmt.Fprintln(os.Stderr, "Succeeded: ", result.Succeeded)
	fmt.Fprintln(os.Stderr, "Failed:    ", result.Failed)
}

func listFiles(directory string) []string {
	files, err := os.ReadDir(directory)
	if err != nil {
		log.Fatal(err)
	}
	return Map(files, func(file fs.DirEntry) string { return filepath.Join(directory, file.Name()) })
}

func Map[T, U any](ts []T, f func(T) U) []U {
	us := make([]U, len(ts))
	for i := range ts {
		us[i] = f(ts[i])
	}
	return us
}
