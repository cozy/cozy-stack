package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/mail"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"

	"github.com/spf13/cobra"
)

var (
	dryRunFlag       bool
	withMetadataFlag bool
)

var fixerCmdGroup = &cobra.Command{
	Use:     "fix <command>",
	Aliases: []string{"fixer"},
	Short:   "A set of tools to fix issues or migrate content.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
	},
}

var mimeFixerCmd = &cobra.Command{
	Use:   "mime <domain>",
	Short: "Fix the class computed from the mime-type",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Usage()
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
			_, class := vfs.ExtractMimeAndClassFromFilename(attrs.Name)
			if class == attrs.Class {
				return nil
			}
			fmt.Fprintf(os.Stdout, "Fix %s: %s -> %s\n", attrs.Name, attrs.Class, class)
			_, err = c.UpdateAttrsByID(doc.ID, &client.FilePatch{
				Rev: doc.Rev,
				Attrs: client.FilePatchAttrs{
					Class: class,
				},
			})
			return err
		})
	},
}

var jobsFixer = &cobra.Command{
	Use:   "jobs <domain>",
	Short: "Take a look at the consistency of the jobs",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Usage()
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

		fmt.Fprintf(os.Stdout, "Cleaned %d jobs on %s\n", result.Deleted, args[0])
		return nil
	},
}

var redisFixer = &cobra.Command{
	Use:   "redis",
	Short: "Rebuild scheduling data strucutures in redis",
	RunE: func(cmd *cobra.Command, args []string) error {
		ac := newAdminClient()
		return ac.RebuildRedis()
	},
}

var thumbnailsFixer = &cobra.Command{
	Use:   "thumbnails <domain>",
	Short: "Rebuild thumbnails image for images files",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return cmd.Usage()
		}
		domain := args[0]
		c := newClient(domain, "io.cozy.jobs")
		res, err := c.JobPush(&client.JobOptions{
			Worker: "thumbnailck",
			Arguments: struct {
				WithMetadata bool `json:"with_metadata"`
			}{
				WithMetadata: withMetadataFlag,
			},
		})
		if err != nil {
			return err
		}
		b, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	},
}

var contactEmailsFixer = &cobra.Command{
	Use:   "contact-emails",
	Short: "Detect and try to fix invalid emails on contacts",
	RunE: func(cmd *cobra.Command, args []string) error {
		ac := newAdminClient()
		instances, err := ac.ListInstances()
		if err != nil {
			return err
		}

		semicolonAtEnd := regexp.MustCompile(";$")
		dotAtStart := regexp.MustCompile(`^\.`)
		dotAtEnd := regexp.MustCompile(`\.$`)
		lessThanStart := regexp.MustCompile("^<")
		greaterThanEnd := regexp.MustCompile(">$")
		mailto := regexp.MustCompile("^mailto:")

		fixEmails := func(domain string) error {
			c, err := newClientSafe(domain, consts.Contacts)
			if err != nil {
				return err
			}

			res, err := c.Req(&request.Options{
				Method: "GET",
				Path:   "/data/" + consts.Contacts + "/_all_docs",
				Queries: url.Values{
					"include_docs": {"true"},
				},
			})
			if err != nil {
				return err
			}
			defer res.Body.Close()

			var contacts struct {
				Rows []struct {
					Contact contact.Contact `json:"doc"`
				} `json:"rows"`
			}
			buf, err := io.ReadAll(res.Body)
			if err != nil {
				return err
			}
			err = json.Unmarshal(buf, &contacts)
			if err != nil {
				return err
			}

			for _, r := range contacts.Rows {
				co := r.Contact
				id := co.ID()
				if strings.HasPrefix(id, "_design") {
					continue
				}

				changed := false
				emails, ok := co.Get("emails").([]interface{})
				if !ok {
					continue
				}
				for i := range emails {
					email, ok := emails[i].(map[string]interface{})
					if !ok {
						continue
					}
					address, ok := email["address"].(string)
					if !ok {
						continue
					}
					_, err := mail.ParseAddress(address)
					if err != nil {
						old := address
						address = strings.TrimSpace(address)
						address = strings.ReplaceAll(address, "\"", "")
						address = strings.ReplaceAll(address, ",", ".")
						address = strings.ReplaceAll(address, " .", ".")
						address = strings.ReplaceAll(address, ". ", ".")
						address = strings.ReplaceAll(address, ". ", ".")
						address = strings.ReplaceAll(address, "..", ".")
						address = strings.ReplaceAll(address, ".@", "@")
						address = strings.ReplaceAll(address, "@.", "@")
						address = strings.ReplaceAll(address, " @", "@")
						address = strings.ReplaceAll(address, "@ ", "@")
						address = mailto.ReplaceAllString(address, "")
						address = semicolonAtEnd.ReplaceAllString(address, "")
						address = dotAtStart.ReplaceAllString(address, "")
						address = dotAtEnd.ReplaceAllString(address, "")
						address = lessThanStart.ReplaceAllString(address, "")
						address = greaterThanEnd.ReplaceAllString(address, "")
						address = strings.TrimSpace(address)
						_, err := mail.ParseAddress(address)
						if err == nil {
							fmt.Fprintf(os.Stdout, "    Email fixed: \"%s\" → \"%s\"\n", old, address)
							changed = true
							email["address"] = address
						} else {
							fmt.Fprintf(os.Stdout, "    Invalid email: \"%s\" → \"%s\"\n", old, address)
						}
					}
				}

				if changed {
					co.M["email"] = emails
					json, err := json.Marshal(co)
					if err != nil {
						return err
					}
					body := bytes.NewReader(json)

					_, err = c.Req(&request.Options{
						Method: "PUT",
						Path:   "/data/" + consts.Contacts + "/" + id,
						Body:   body,
					})
					if err != nil {
						return err
					}
				}
			}

			return nil
		}

		for _, instance := range instances {
			domain := instance.Attrs.Domain
			fmt.Fprintf(os.Stderr, "Fixing %s contact emails...\n", domain)
			err := fixEmails(domain)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error occurred: %s\n", err)
			}
		}

		return nil
	},
}

var passwordDefinedFixer = &cobra.Command{
	Use:   "password-defined <domain>",
	Short: "Set the password_defined setting",
	Long: `
A password_defined field has been added to the io.cozy.settings.instance
(available on GET /settings/instance). This fixer can fill it for existing Cozy
instances if it was missing.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return cmd.Usage()
		}
		domain := args[0]
		c := newAdminClient()
		path := fmt.Sprintf("/instances/%s/fixers/password-defined", domain)
		_, err := c.Req(&request.Options{
			Method: "POST",
			Path:   path,
		})
		return err
	},
}

var orphanAccountFixer = &cobra.Command{
	Use:   "orphan-account <domain>",
	Short: "Remove the orphan accounts",
	Long: `
This fixer detects the accounts that are linked to a konnector that has been
uninstalled, and then removed them.

For banking accounts, the konnector must run to also clean the account
remotely. To do so, the konnector is installed, the account is deleted,
the stack runs the konnector with the AccountDeleted flag, and when it's
done, the konnector is uninstalled again.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return cmd.Usage()
		}
		domain := args[0]
		c := newAdminClient()
		path := fmt.Sprintf("/instances/%s/fixers/orphan-account", domain)
		_, err := c.Req(&request.Options{
			Method: "POST",
			Path:   path,
		})
		return err
	},
}

var serviceTriggersFixer = &cobra.Command{
	Use:   "service-triggers <domain>",
	Short: "Clean the triggers for webapp services",
	Long: `
This fixer cleans duplicate triggers for webapp services, and recreates missing
triggers.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return cmd.Usage()
		}
		domain := args[0]
		c := newAdminClient()
		path := fmt.Sprintf("/instances/%s/fixers/service-triggers", domain)
		res, err := c.Req(&request.Options{
			Method: "POST",
			Path:   path,
		})
		if err != nil {
			return err
		}
		out, err := io.ReadAll(res.Body)
		if err != nil {
			return err
		}
		fmt.Print(string(out))
		return nil
	},
}

var indexesFixer = &cobra.Command{
	Use:   "indexes <domain>",
	Short: "Rebuild the CouchDB views and indexes",
	Long: `
This fixer ensures that the CouchDB views and indexes used by the stack for
this instance are correctly set.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return cmd.Usage()
		}
		domain := args[0]
		c := newAdminClient()
		path := fmt.Sprintf("/instances/%s/fixers/indexes", domain)
		_, err := c.Req(&request.Options{
			Method: "POST",
			Path:   path,
		})
		return err
	},
}

func init() {
	thumbnailsFixer.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Dry run")
	thumbnailsFixer.Flags().BoolVar(&withMetadataFlag, "with-metadata", false, "Recalculate images metadata")

	fixerCmdGroup.AddCommand(jobsFixer)
	fixerCmdGroup.AddCommand(mimeFixerCmd)
	fixerCmdGroup.AddCommand(redisFixer)
	fixerCmdGroup.AddCommand(thumbnailsFixer)
	fixerCmdGroup.AddCommand(contactEmailsFixer)
	fixerCmdGroup.AddCommand(passwordDefinedFixer)
	fixerCmdGroup.AddCommand(orphanAccountFixer)
	fixerCmdGroup.AddCommand(serviceTriggersFixer)
	fixerCmdGroup.AddCommand(indexesFixer)

	RootCmd.AddCommand(fixerCmdGroup)
}
