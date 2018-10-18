package cmd

// #nosec
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
	"strings"

	"github.com/cozy/cozy-stack/pkg/contacts"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/spf13/cobra"
)

var dryRunFlag bool
var withMetadataFlag bool

var fixerCmdGroup = &cobra.Command{
	Use:   "fixer <command>",
	Short: "A set of tools to fix issues or migrate content for retro-compatibility.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
	},
}

var albumsCreatedAtFixerCmd = &cobra.Command{
	Use:   "albums-created-at <domain>",
	Short: "Add a created_at field for albums where it's missing",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Usage()
		}
		c := newClient(args[0], consts.PhotosAlbums)
		res, err := c.Req(&request.Options{
			Method: "GET",
			Path:   "/data/" + consts.PhotosAlbums + "/_all_docs",
			Queries: url.Values{
				"limit":        {"1000"},
				"include_docs": {"true"},
			},
		})
		if err != nil {
			return err
		}
		defer res.Body.Close()
		var result map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&result)
		if err != nil {
			return err
		}
		rows, ok := result["rows"].([]interface{})
		if !ok {
			return nil // no albums
		}

		count := 0
		for _, r := range rows {
			row := r.(map[string]interface{})
			id := row["id"].(string)
			if strings.HasPrefix(id, "_design") {
				continue
			}
			album := row["doc"].(map[string]interface{})
			if _, ok := album["created_at"]; ok {
				continue
			}
			count++
			album["created_at"] = "2017-06-01T02:03:04.000Z"
			buf, err := json.Marshal(album)
			if err != nil {
				return err
			}
			body := bytes.NewReader(buf)
			_, err = c.Req(&request.Options{
				Method: "PUT",
				Path:   "/data/" + consts.PhotosAlbums + "/" + id,
				Body:   body,
			})
			if err != nil {
				return err
			}
		}

		fmt.Printf("Added created_at for %d albums on %s\n", count, args[0])
		return nil
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
			h := md5.New() // #nosec
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

var onboardingsFixer = &cobra.Command{
	Use:   "onboardings",
	Short: "Add the onboarding_finished flag to user that have registered their passphrase",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newAdminClient()
		list, err := c.ListInstances()
		if err != nil {
			return err
		}
		var hasErrored bool
		t := true
		for _, i := range list {
			if len(i.Attrs.RegisterToken) > 0 || i.Attrs.OnboardingFinished {
				continue
			}
			fmt.Printf("Setting onboarding finished flag on '%s'...", i.Attrs.Domain)
			_, err = c.ModifyInstance(&client.InstanceOptions{
				Domain:             i.Attrs.Domain,
				OnboardingFinished: &t,
			})
			if err != nil {
				fmt.Printf("failed: %s\n", err)
				hasErrored = true
			} else {
				fmt.Printf("ok\n")
			}
		}
		if hasErrored {
			os.Exit(1)
		}
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
					Contact contacts.Contact `json:"doc"`
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
				contact := r.Contact
				id := contact.ID()
				if strings.HasPrefix(id, "_design") {
					continue
				}

				changed := false
				for _, email := range contact.Email {
					address := email.Address
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
							email.Address = address
						} else {
							fmt.Printf("    Invalid email: \"%s\" → \"%s\"\n", old, address)
						}
					}
				}

				if changed {
					json, err := json.Marshal(contact)
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
				fmt.Fprintf(os.Stderr, "Error occured: %s\n", err)
			}
		}

		return nil
	},
}

func init() {

	thumbnailsFixer.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Dry run")
	thumbnailsFixer.Flags().BoolVar(&withMetadataFlag, "with-metadata", false, "Recalculate images metadata")

	fixerCmdGroup.AddCommand(albumsCreatedAtFixerCmd)
	fixerCmdGroup.AddCommand(jobsFixer)
	fixerCmdGroup.AddCommand(md5FixerCmd)
	fixerCmdGroup.AddCommand(mimeFixerCmd)
	fixerCmdGroup.AddCommand(onboardingsFixer)
	fixerCmdGroup.AddCommand(redisFixer)
	fixerCmdGroup.AddCommand(thumbnailsFixer)
	fixerCmdGroup.AddCommand(contactEmailsFixer)

	RootCmd.AddCommand(fixerCmdGroup)
}
