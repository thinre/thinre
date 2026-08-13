// Package integrationspec defines the Integration v1 contract: the YAML
// schema describing how the Thinre Supervisor manages a specific black-box
// software product (upgrade/rollback/version hooks, configuration files,
// health checks), plus its parser and validator.
//
// The validator is implemented once, here, and imported by both the
// Supervisor and Thinre Cloud so validation behaves identically at the edge
// and in the cloud.
package integrationspec
