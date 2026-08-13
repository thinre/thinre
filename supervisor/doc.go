// Package supervisor contains the Thinre Supervisor's domain logic, split by
// responsibility into subpackages: reconcile (desired-vs-observed state
// machine), artifacts (download and verification), hooks (lifecycle hook
// execution), health (health checks), identity (machine identity), and state
// (crash-safe local state persistence).
package supervisor
