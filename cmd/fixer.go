package cmd

// #nosec
import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/spf13/cobra"
)

var fixerCmdGroup = &cobra.Command{
	Use:   "fixer [command]",
	Short: "A set of tools to fix issues or migrate content for retro-compatibility.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var md5FixerCmd = &cobra.Command{
	Use:   "md5 [domain]",
	Short: "Fix missing md5 from contents in the vfs",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		c := newClient(args[0], consts.Files)
		return c.WalkByPath("/", func(name string, doc *client.DirOrFile, err error) error {
			if err != nil {
				return err
			}
			attrs := doc.Attrs
			if attrs.Type == consts.DirType {
				return nil
			}
			if len(attrs.MD5Sum) > 0 {
				return nil
			}
			fmt.Printf("Recalculate md5 of %s...", name)
			r, err := c.DownloadByID(doc.ID)
			if err != nil {
				return err
			}
			defer r.Close()
			h := md5.New() // #nosec
			_, err = io.Copy(h, r)
			if err != nil {
				return err
			}
			_, err = c.UpdateAttrsByID(doc.ID, &client.FilePatch{
				Rev: doc.Rev,
				Attrs: client.FilePatchAttrs{
					MD5Sum: h.Sum(nil),
				},
			})
			if err != nil {
				return err
			}
			fmt.Println()
			return nil
		})
	},
}

var triggersFixer = &cobra.Command{
	Use:   "triggers [domain]",
	Short: "Remove orphaned triggers from an instance",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		c := newClient(args[0], consts.Triggers+" "+consts.Accounts)
		res, err := c.Req(&request.Options{
			Method: "POST",
			Path:   "/jobs/triggers/clean",
		})
		if err != nil {
			return err
		}
		defer res.Body.Close()
		var result struct {
			Deleted int `json:"deleted"`
		}
		err = json.NewDecoder(res.Body).Decode(&result)
		if err != nil {
			return err
		}

		fmt.Printf("Cleaned %d orphans\n", result.Deleted)
		return nil
	},
}

var jobsFixer = &cobra.Command{
	Use:   "jobs [domain]",
	Short: "Take a look at the consistency of the jobs",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		c := newClient(args[0], consts.Jobs)
		res, err := c.Req(&request.Options{
			Method: "POST",
			Path:   "/jobs/clean",
		})
		if err != nil {
			return err
		}
		defer res.Body.Close()
		var result struct {
			Deleted int `json:"deleted"`
		}
		err = json.NewDecoder(res.Body).Decode(&result)
		if err != nil {
			return err
		}

		fmt.Printf("Cleaned %d jobs\n", result.Deleted)
		return nil
	},
}

func init() {
	fixerCmdGroup.AddCommand(md5FixerCmd)
	fixerCmdGroup.AddCommand(triggersFixer)
	fixerCmdGroup.AddCommand(jobsFixer)
	RootCmd.AddCommand(fixerCmdGroup)
}
