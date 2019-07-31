package cmd

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/mail"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/stack"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"

	"github.com/spf13/cobra"
)

var dryRunFlag bool
var withMetadataFlag bool
var noDryRunFlag bool

var fixerCmdGroup = &cobra.Command{
	Use:   "fixer <command>",
	Short: "A set of tools to fix issues or migrate content for retro-compatibility.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
	},
}

var md5FixerCmd = &cobra.Command{
	Use:   "md5 <domain>",
	Short: "Fix missing md5 from contents in the vfs",
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
			if len(attrs.MD5Sum) > 0 {
				return nil
			}
			fmt.Printf("Recalculate md5 of %s...", name)
			r, err := c.DownloadByID(doc.ID)
			if err != nil {
				fmt.Printf("failed to init download: %s", err.Error())
				return nil
			}
			defer r.Close()
			h := md5.New()
			_, err = io.Copy(h, r)
			if err != nil {
				fmt.Printf("failed to download: %s", err.Error())
				return nil
			}
			_, err = c.UpdateAttrsByID(doc.ID, &client.FilePatch{
				Rev: doc.Rev,
				Attrs: client.FilePatchAttrs{
					MD5Sum: h.Sum(nil),
				},
			})
			if err != nil {
				fmt.Printf("failed to update: %s", err.Error())
				return nil
			}
			fmt.Println("ok.")
			return nil
		})
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
			fmt.Printf("Fix %s: %s -> %s\n", attrs.Name, attrs.Class, class)
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

		fmt.Printf("Cleaned %d jobs on %s\n", result.Deleted, args[0])
		return nil
	},
}

var redisFixer = &cobra.Command{
	Use:   "redis",
	Short: "Rebuild scheduling data strucutures in redis",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newAdminClient()
		return c.RebuildRedis()
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
		c := newAdminClient()
		instances, err := c.ListInstances()
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
			buf, err := ioutil.ReadAll(res.Body)
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
						address = strings.Replace(address, "\"", "", -1)
						address = strings.Replace(address, ",", ".", -1)
						address = strings.Replace(address, " .", ".", -1)
						address = strings.Replace(address, ". ", ".", -1)
						address = strings.Replace(address, ". ", ".", -1)
						address = strings.Replace(address, "..", ".", -1)
						address = strings.Replace(address, ".@", "@", -1)
						address = strings.Replace(address, "@.", "@", -1)
						address = strings.Replace(address, " @", "@", -1)
						address = strings.Replace(address, "@ ", "@", -1)
						address = mailto.ReplaceAllString(address, "")
						address = semicolonAtEnd.ReplaceAllString(address, "")
						address = dotAtStart.ReplaceAllString(address, "")
						address = dotAtEnd.ReplaceAllString(address, "")
						address = lessThanStart.ReplaceAllString(address, "")
						address = greaterThanEnd.ReplaceAllString(address, "")
						address = strings.TrimSpace(address)
						_, err := mail.ParseAddress(address)
						if err == nil {
							fmt.Printf("    Email fixed: \"%s\" → \"%s\"\n", old, address)
							changed = true
							email["address"] = address
						} else {
							fmt.Printf("    Invalid email: \"%s\" → \"%s\"\n", old, address)
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

var contentMismatch64Kfixer = &cobra.Command{
	Use:   "content-mismatch <domain>",
	Short: "Fix the content mismatch differences for 64K issue",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Usage()
		}

		domain := args[0]
		q := url.Values{
			"dry_run": {strconv.FormatBool(!noDryRunFlag)},
		}

		c := newAdminClient()
		res, err := c.Req(&request.Options{
			Method:  "GET",
			Path:    "/instances/" + url.PathEscape(domain) + "/content-mismatch-fixer",
			Queries: q,
		})
		if err != nil {
			return err
		}

		out, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}
		fmt.Println(string(out))

		return nil
	},
}

var orphanAccountFixer = &cobra.Command{
	Use:   "orphan-account <domain>",
	Short: "Rebuild scheduling data strucutures in redis",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return cmd.Usage()
		}

		domain := args[0]
		inst, err := lifecycle.GetInstance(domain)
		if err != nil {
			return err
		}

		var accounts []*account.Account
		err = couchdb.GetAllDocs(inst, consts.Accounts, nil, &accounts)
		if err != nil || len(accounts) == 0 {
			return err
		}

		var konnectors []*app.KonnManifest
		err = couchdb.GetAllDocs(inst, consts.Konnectors, nil, &konnectors)
		if err != nil {
			return err
		}

		var slugsToDelete []string
		for _, acc := range accounts {
			if acc.AccountType == "" {
				continue // Skip the design docs
			}
			found := false
			for _, konn := range konnectors {
				if konn.Slug() == acc.AccountType {
					found = true
					break
				}
			}
			if !found {
				for _, slug := range slugsToDelete {
					if slug == acc.AccountType {
						found = true
						break
					}
				}
				if !found {
					slugsToDelete = append(slugsToDelete, acc.AccountType)
				}
			}
		}
		if len(slugsToDelete) == 0 {
			return nil
		}

		if _, err = stack.Start(); err != nil {
			return err
		}
		jobsSystem := job.System()
		log := inst.Logger().WithField("nspace", "fixer")
		copier := inst.AppsCopier(consts.KonnectorType)

		for _, slug := range slugsToDelete {
			opts := &app.InstallerOptions{
				Operation:  app.Install,
				Type:       consts.KonnectorType,
				SourceURL:  "registry://" + slug,
				Slug:       slug,
				Registries: inst.Registries(),
			}
			ins, err := app.NewInstaller(inst, copier, opts)
			if err != nil {
				return err
			}
			if _, err = ins.RunSync(); err != nil {
				return err
			}

			for _, acc := range accounts {
				if acc.AccountType != slug {
					continue
				}
				acc.ManualCleaning = true
				if err := couchdb.DeleteDoc(inst, acc); err != nil {
					log.Errorf("Cannot delete account: %v", err)
				}
				j, err := account.PushAccountDeletedJob(jobsSystem, inst, acc.ID(), acc.Rev(), slug)
				if err != nil {
					log.Errorf("Cannot push a job for account deletion: %v", err)
				}
				if err = j.WaitUntilDone(inst); err != nil {
					log.Error(err)
				}
			}
			opts.Operation = app.Delete
			ins, err = app.NewInstaller(inst, copier, opts)
			if err != nil {
				return err
			}
			if _, err = ins.RunSync(); err != nil {
				return err
			}
		}

		return nil
	},
}

func init() {

	thumbnailsFixer.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Dry run")
	thumbnailsFixer.Flags().BoolVar(&withMetadataFlag, "with-metadata", false, "Recalculate images metadata")
	contentMismatch64Kfixer.Flags().BoolVar(&noDryRunFlag, "no-dry-run", false, "Do not dry run")

	fixerCmdGroup.AddCommand(jobsFixer)
	fixerCmdGroup.AddCommand(md5FixerCmd)
	fixerCmdGroup.AddCommand(mimeFixerCmd)
	fixerCmdGroup.AddCommand(redisFixer)
	fixerCmdGroup.AddCommand(thumbnailsFixer)
	fixerCmdGroup.AddCommand(contactEmailsFixer)
	fixerCmdGroup.AddCommand(contentMismatch64Kfixer)
	fixerCmdGroup.AddCommand(orphanAccountFixer)

	RootCmd.AddCommand(fixerCmdGroup)
}
