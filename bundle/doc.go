// Package bundle defines the configuration bundle manifest format: a
// versioned set of one or more configuration files that is staged, verified,
// and applied atomically by the Supervisor. A bundle revision is the unit of
// desired configuration state; it is never reported active until every file
// in the revision is staged and applied.
package bundle
