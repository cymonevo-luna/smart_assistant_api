// Package deploy holds tests for the repository's deploy-time shell scripts
// (scripts/*.sh). The scripts run on the deploy host, but their branching logic
// is exercised here through the scripts' documented test hooks so regressions
// (e.g. a deploy gate swallowing errors) are caught by `go test`.
package deploy
