// Package builtins registers every connector compiled into the server binary.
//
// To add a new built-in connector, add a `connectors.Register(...)` call here
// in init(). At phase 7 we add a separate plugins/ mechanism for community
// connectors that ship as their own modules and load via init() side-effect.
package builtins

import (
	"github.com/sunny/sunny/apps/server/internal/connectors"

	mqtt "github.com/sunny/sunny/connectors/mqtt"
	nasafirms "github.com/sunny/sunny/connectors/nasafirms"
	noaaweather "github.com/sunny/sunny/connectors/noaaweather"
	openaq "github.com/sunny/sunny/connectors/openaq"
	postgres "github.com/sunny/sunny/connectors/postgres"
	usgsearthquakes "github.com/sunny/sunny/connectors/usgsearthquakes"
	usgswater "github.com/sunny/sunny/connectors/usgswater"
	webhook "github.com/sunny/sunny/connectors/webhook"
	hello "github.com/sunny/sunny/packages/sdk-go/example_hello"
)

func init() {
	connectors.Register(hello.New())
	connectors.Register(usgsearthquakes.New())
	connectors.Register(noaaweather.New())
	connectors.Register(nasafirms.New())
	connectors.Register(usgswater.New())
	connectors.Register(openaq.New())
	connectors.Register(webhook.New())
	connectors.Register(mqtt.New())
	connectors.Register(postgres.New())
}
