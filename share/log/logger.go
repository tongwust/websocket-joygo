package log

import (
	"encoding/json"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewZapLogger() (*zap.Logger, error) {
	rawJSON := []byte(`{
	  "level": "debug",
	  "encoding": "json",
	  "outputPaths": ["stdout","/tmp/logs/info.log"],
      "errorOutputPaths": ["stderr","/tmp/logs/err.log"],
	  "encoderConfig": {
	    "timeKey": "time",
		"messageKey":"message",
	    "levelKey": "level",
	    "levelEncoder": "lowercase"
	  }
	}`)
	var cfg zap.Config
	if err := json.Unmarshal(rawJSON, &cfg); err != nil {
		panic(err)
	}
	//cfg := zap.NewDevelopmentConfig()
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		//zapcore.TimeEncoderOfLayout("2006-01-02 15:01:02")
	return cfg.Build()
}
