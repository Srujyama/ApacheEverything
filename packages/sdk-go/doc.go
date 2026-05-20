// Package sdk is the Sunny connector SDK for Go.
//
// A connector is a plugin that pulls or receives data from an external source
// (USGS API, NOAA feed, an MQTT broker, a Postgres CDC stream, ...) and
// publishes records into the Sunny ingest pipeline.
//
// # Versioning
//
// This package is now at v1. The exported surface — types Mode, Category,
// Manifest, Record, GeoPoint, Logger, Connector, Context, PushHandler, and
// the Mode/Category constants — is FROZEN. Any change that removes or
// renames a field, changes a method signature, or changes the JSON shape
// of a wire type is a BREAKING change and requires a v2 module path.
//
// Additive changes (new categories, new optional fields on Manifest, new
// methods on a NEW interface) are allowed under v1.
//
// The sdk_freeze_test.go file in this package uses reflection to enforce
// the freeze: any modification to the listed types fails the test and the
// CI build, forcing the author to either revert or open a v2.
//
// # Wire format compatibility
//
// Anything tagged with `json:"..."` is part of the public wire contract —
// it appears in HTTP responses, persisted checkpoints, marketplace
// manifests, and inter-connector messages. Tag changes are breaking even
// if the Go-level field name is unchanged.
//
// # SDKVersion
//
// SDKVersion is a string the runtime reports in logs and metrics. It is
// bumped on every release of the sdk package.
package sdk

// SDKVersion is the semver of this SDK module. Bumped on every release.
//
// Increment rules (apply to SDK only — the server binary has its own
// versioning):
//
//   - MAJOR: anything sdk_freeze_test.go disallows (breaking).
//   - MINOR: a new exported type, constant, or method on a new interface.
//   - PATCH: doc, bug fix, or non-API-visible change.
const SDKVersion = "1.0.0"
