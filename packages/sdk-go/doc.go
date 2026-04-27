// Package sdk is the Sunny connector SDK for Go.
//
// A connector is a plugin that pulls or receives data from an external source
// (USGS API, NOAA feed, an MQTT broker, a Postgres CDC stream, ...) and
// publishes records into the Sunny ingest pipeline.
//
// Phase 0 ships only the package marker. Phase 1 fills in the Connector
// interface, ConnectorContext, and the three modes (Pull, Push, Stream).
package sdk
