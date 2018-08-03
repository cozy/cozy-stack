package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/spf13/cobra"
)

var flagJobJSONArg string
var flagJobPrintLogs bool
var flagJobPrintLogsVerbose bool

var jobsCmdGroup = &cobra.Command{
	Use:   "jobs <command>",
	Short: "Launch and manage jobs and workers",
}

var jobsRunCmd = &cobra.Command{
	Use:     "run <worker>",
	Aliases: []string{"launch", "push"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return cmd.Help()
		}
		if flagDomain == "" {
			return errAppsMissingDomain
		}
		c := newClient(flagDomain, "io.cozy.jobs", "io.cozy.jobs.logs")
		o := &client.JobOptions{
			Worker:    args[0],
			Arguments: json.RawMessage(flagJobJSONArg),
		}
		if flagJobPrintLogs {
			o.Logs = make(chan *client.JobLog)
			go func() {
				for log := range o.Logs {
					fmt.Printf("[%s]", log.Level)
					if flagJobPrintLogsVerbose {
						fmt.Printf("[time:%s]", log.Time.Format(time.RFC3339))
						for k, v := range log.Data {
							fmt.Printf("[%s:%s]", k, v)
						}
					}
					fmt.Printf(" %s\n", log.Message)
				}
			}()
		}
		j, err := c.JobPush(o)
		if err != nil {
			return err
		}
		b, err := json.MarshalIndent(j, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	},
}

func init() {
	domain := os.Getenv("COZY_DOMAIN")
	if domain == "" && config.IsDevRelease() {
		domain = defaultDevDomain
	}

	jobsCmdGroup.PersistentFlags().StringVar(&flagDomain, "domain", domain, "specify the domain name of the instance")

	jobsRunCmd.Flags().StringVar(&flagJobJSONArg, "json", "", "specify the job arguments as raw JSON")
	jobsRunCmd.Flags().BoolVar(&flagJobPrintLogs, "logs", false, "print jobs log in stdout")
	jobsRunCmd.Flags().BoolVar(&flagJobPrintLogsVerbose, "logs-verbose", false, "verbose logging (with --logs flag)")

	jobsCmdGroup.AddCommand(jobsRunCmd)
	RootCmd.AddCommand(jobsCmdGroup)
}
