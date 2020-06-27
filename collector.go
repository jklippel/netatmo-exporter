package main

import (
	"sync"
	"time"

	netatmo "github.com/exzz/netatmo-api-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

var (
	prefix        = "netatmo_"
	netatmoUpDesc = prometheus.NewDesc(prefix+"up",
		"Zero if there was an error during the last refresh try.",
		nil, nil)

	refreshPrefix        = prefix + "last_refresh"
	refreshTimestampDesc = prometheus.NewDesc(
		refreshPrefix+"_time",
		"Contains the time of the last refresh try, successful or not.",
		nil, nil)

	cacheTimestampDesc = prometheus.NewDesc(
		prefix+"cache_updated_time",
		"Contains the time of the cached data.",
		nil, nil)

	varLabels = []string{
		"module",
		"station",
	}

	sensorPrefix = prefix + "sensor_"

	updatedDesc = prometheus.NewDesc(
		sensorPrefix+"updated",
		"Timestamp of last update",
		varLabels,
		nil)

	tempDesc = prometheus.NewDesc(
		sensorPrefix+"temperature_celsius",
		"Temperature measurement in celsius",
		varLabels,
		nil)

	humidityDesc = prometheus.NewDesc(
		sensorPrefix+"humidity_percent",
		"Relative humidity measurement in percent",
		varLabels,
		nil)

	cotwoDesc = prometheus.NewDesc(
		sensorPrefix+"co2_ppm",
		"Carbondioxide measurement in parts per million",
		varLabels,
		nil)

	noiseDesc = prometheus.NewDesc(
		sensorPrefix+"noise_db",
		"Noise measurement in decibels",
		varLabels,
		nil)

	pressureDesc = prometheus.NewDesc(
		sensorPrefix+"pressure_mb",
		"Atmospheric pressure measurement in millibar",
		varLabels,
		nil)

	windStrengthDesc = prometheus.NewDesc(
		sensorPrefix+"wind_strength_kph",
		"Wind strength in kilometers per hour",
		varLabels,
		nil)

	windDirectionDesc = prometheus.NewDesc(
		sensorPrefix+"wind_direction_degrees",
		"Wind direction in degrees",
		varLabels,
		nil)

	rainDesc = prometheus.NewDesc(
		sensorPrefix+"rain_amount_mm",
		"Rain amount in millimeters",
		varLabels,
		nil)

	batteryDesc = prometheus.NewDesc(
		sensorPrefix+"battery_percent",
		"Battery remaining life (10: low)",
		varLabels,
		nil)
	wifiDesc = prometheus.NewDesc(
		sensorPrefix+"wifi_signal_strength",
		"Wifi signal strength (86: bad, 71: avg, 56: good)",
		varLabels,
		nil)
	rfDesc = prometheus.NewDesc(
		sensorPrefix+"rf_signal_strength",
		"RF signal strength (90: lowest, 60: highest)",
		varLabels,
		nil)
)

type netatmoCollector struct {
	log              logrus.FieldLogger
	refreshInterval  time.Duration
	staleThreshold   time.Duration
	client           *netatmo.Client
	lastRefresh      time.Time
	lastRefreshError error
	cacheLock        sync.RWMutex
	cacheTimestamp   time.Time
	cachedData       *netatmo.DeviceCollection
}

func (c *netatmoCollector) Describe(dChan chan<- *prometheus.Desc) {
	dChan <- updatedDesc
	dChan <- tempDesc
	dChan <- humidityDesc
	dChan <- cotwoDesc
}

func (c *netatmoCollector) Collect(mChan chan<- prometheus.Metric) {
	now := time.Now()
	if now.Sub(c.lastRefresh) >= c.refreshInterval {
		go c.refreshData(now)
	}

	upValue := 1.0
	if c.lastRefresh.IsZero() || c.lastRefreshError != nil {
		upValue = 0
	}
	c.sendMetric(mChan, netatmoUpDesc, prometheus.GaugeValue, upValue)
	c.sendMetric(mChan, refreshTimestampDesc, prometheus.GaugeValue, convertTime(c.lastRefresh))

	c.cacheLock.RLock()
	defer c.cacheLock.RUnlock()

	c.sendMetric(mChan, cacheTimestampDesc, prometheus.GaugeValue, convertTime(c.cacheTimestamp))
	if c.cachedData != nil {
		for _, dev := range c.cachedData.Devices() {
			stationName := dev.StationName
			c.collectData(mChan, dev, stationName)

			for _, module := range dev.LinkedModules {
				c.collectData(mChan, module, stationName)
			}
		}
	}
}

func (c *netatmoCollector) refreshData(now time.Time) {
	c.log.Debugf("Refresh interval elapsed: %s > %s", now.Sub(c.lastRefresh), c.refreshInterval)
	c.lastRefresh = now

	devices, err := c.client.Read()
	if err != nil {
		c.log.Errorf("Error during refresh: %s", err)
		c.lastRefreshError = err
		return
	}

	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()
	c.cacheTimestamp = now
	c.cachedData = devices
}

func (c *netatmoCollector) collectData(ch chan<- prometheus.Metric, device *netatmo.Device, stationName string) {
	moduleName := device.ModuleName
	data := device.DashboardData

	if data.LastMeasure == nil {
		c.log.Debugf("No data available.")
		return
	}

	date := time.Unix(*data.LastMeasure, 0)
	if time.Since(date) > c.staleThreshold {
		c.log.Debugf("Data is stale for %s: %s > %s", moduleName, time.Since(date), c.staleThreshold)
		return
	}

	c.sendMetric(ch, updatedDesc, prometheus.CounterValue, float64(date.UTC().Unix()), moduleName, stationName)

	if data.Temperature != nil {
		c.sendMetric(ch, tempDesc, prometheus.GaugeValue, float64(*data.Temperature), moduleName, stationName)
	}

	if data.Humidity != nil {
		c.sendMetric(ch, humidityDesc, prometheus.GaugeValue, float64(*data.Humidity), moduleName, stationName)
	}

	if data.CO2 != nil {
		c.sendMetric(ch, cotwoDesc, prometheus.GaugeValue, float64(*data.CO2), moduleName, stationName)
	}

	if data.Noise != nil {
		c.sendMetric(ch, noiseDesc, prometheus.GaugeValue, float64(*data.Noise), moduleName, stationName)
	}

	if data.Pressure != nil {
		c.sendMetric(ch, pressureDesc, prometheus.GaugeValue, float64(*data.Pressure), moduleName, stationName)
	}

	if data.WindStrength != nil {
		c.sendMetric(ch, windStrengthDesc, prometheus.GaugeValue, float64(*data.WindStrength), moduleName, stationName)
	}

	if data.WindAngle != nil {
		c.sendMetric(ch, windDirectionDesc, prometheus.GaugeValue, float64(*data.WindAngle), moduleName, stationName)
	}

	if data.Rain != nil {
		c.sendMetric(ch, rainDesc, prometheus.GaugeValue, float64(*data.Rain), moduleName, stationName)
	}

	if device.BatteryPercent != nil {
		c.sendMetric(ch, batteryDesc, prometheus.GaugeValue, float64(*device.BatteryPercent), moduleName, stationName)
	}
	if device.WifiStatus != nil {
		c.sendMetric(ch, wifiDesc, prometheus.GaugeValue, float64(*device.WifiStatus), moduleName, stationName)
	}
	if device.RFStatus != nil {
		c.sendMetric(ch, rfDesc, prometheus.GaugeValue, float64(*device.RFStatus), moduleName, stationName)
	}
}

func (c *netatmoCollector) sendMetric(ch chan<- prometheus.Metric, desc *prometheus.Desc, valueType prometheus.ValueType, value float64, labelValues ...string) {
	m, err := prometheus.NewConstMetric(desc, valueType, value, labelValues...)
	if err != nil {
		c.log.Errorf("Error creating %s metric: %s", updatedDesc.String(), err)
		return
	}
	ch <- m
}

func convertTime(t time.Time) float64 {
	if t.IsZero() {
		return 0.0
	}

	return float64(t.Unix())
}
