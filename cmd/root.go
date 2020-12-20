/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"flag"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"time"
)

var cfgFile string
var debug bool

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:          "spiderhouse",
	Short:        "Spiderhouse catch your dump from database_url and store it in its aws bucket house",
	SilenceUsage: true,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {

		zerolog.TimestampFunc = func() time.Time {
			loc, err := time.LoadLocation("Europe/paris")
			if err != nil {
				panic(err)
			}
			return time.Now().In(loc)
		}
		debug := flag.Bool("debug", false, "sets log level to debug")
		flag.Parse()
		// Default level for this example is info, unless debug flag is present
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		if *debug {
			zerolog.SetGlobalLevel(zerolog.DebugLevel)
		}


		log.Logger = zerolog.New(os.Stderr).With().Caller().Timestamp().Logger()
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	//rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.spiderhouse.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "Print debug informations")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			log.Error().Msgf("%s", err)
		}

		// Search config in home directory with name ".spiderhouse" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".spiderhouse")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		log.Printf("Using config file:", viper.ConfigFileUsed())
	}
}

func timeNow(name string) time.Time {
	loc, err := time.LoadLocation("Europe/paris")
	if err != nil {
		panic(err)
	}
	return time.Now().In(loc)
}