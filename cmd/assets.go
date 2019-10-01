package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/cozy/cozy-stack/client/request"
	modelAsset "github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/spf13/cobra"
)

var flagURL string
var flagName string
var flagShasum string
var flagContext string

var assetsCmdGroup = &cobra.Command{
	Use:   "assets <command>",
	Short: "Show and manage dynamic assets",
	Long:  `cozy-stack assets can be used to list, insert or remove dynamic assets`,
}

var addAssetCmd = &cobra.Command{
	Use:     "add --url <url> --name <name> --shasum <shasum> --context <context>",
	Aliases: []string{"insert"},
	Short:   "Insert an asset",
	Example: "$ cozy-stack assets add --url file:///foo/bar/baz.js --name /foo/bar/baz.js --shasum 0763d6c2cebee0880eb3a9cc25d38cd23db39b5c3802f2dc379e408c877a2788 --context foocontext",
	RunE:    addAsset,
}

func addAsset(cmd *cobra.Command, args []string) error {
	if flagContext == "" {
		return fmt.Errorf("You must provide a context")
	}

	assetOption := modelAsset.AssetOption{
		URL:     flagURL,
		Name:    flagName,
		Shasum:  flagShasum,
		Context: flagContext,
	}

	customAssets := []modelAsset.AssetOption{assetOption}
	marshaledAssets, err := json.Marshal(customAssets)
	if err != nil {
		return err
	}

	c := newAdminClient()
	req := &request.Options{
		Method: "POST",
		Path:   "instances/assets",
		Body:   bytes.NewReader(marshaledAssets),
	}
	res, err := c.Req(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return nil
}

var rmAssetCmd = &cobra.Command{
	Use:     "rm [context] [name]",
	Aliases: []string{"remove"},
	Short:   "Removes an asset",
	Long:    "Removes a custom asset in a specific context",
	Example: "$ cozy-stack config rm-asset foobar /foo/bar/baz.js",
	RunE:    rmAsset,
}

func rmAsset(cmd *cobra.Command, args []string) error {
	// Check params
	if len(args) != 2 {
		return cmd.Usage()
	}

	c := newAdminClient()
	req := &request.Options{
		Method: "DELETE",
		Path:   fmt.Sprintf("instances/assets/%s/%s", args[0], args[1]),
	}
	res, err := c.Req(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return nil
}

var lsAssetsCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List assets",
	Long:    "List assets currently served by the stack",
	Example: "$ cozy-stack config ls-assets",
	RunE:    lsAssets,
}

func lsAssets(cmd *cobra.Command, args []string) error {
	c := newAdminClient()
	req := &request.Options{
		Method: "GET",
		Path:   "instances/assets",
	}
	res, err := c.Req(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	var v interface{}
	err = json.NewDecoder(res.Body).Decode(&v)
	if err != nil {
		return err
	}

	json, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(json))
	return nil
}

func init() {
	assetsCmdGroup.AddCommand(addAssetCmd)
	assetsCmdGroup.AddCommand(lsAssetsCmd)
	assetsCmdGroup.AddCommand(rmAssetCmd)
	RootCmd.AddCommand(assetsCmdGroup)
	addAssetCmd.Flags().StringVar(&flagURL, "url", "", "The URL of the asset")
	addAssetCmd.Flags().StringVar(&flagName, "name", "", "The name of the asset")
	addAssetCmd.Flags().StringVar(&flagShasum, "shasum", "", "The shasum of the asset")
	addAssetCmd.Flags().StringVar(&flagContext, "context", "", "The context of the asset")
}
