package config

import (
	"fmt"

	"github.com/tkanos/gonfig"
)

// Config structure
type Config struct {
	Region           string
	OrganizationRole string
	MasterAccountID  string
}

// InitVariables based on json file
func InitVariables(input ...string) Config {
	configuration := Config{}
	fileName := fmt.Sprintf("./config/default.json")
	gonfig.GetConf(fileName, &configuration)

	return configuration
}
